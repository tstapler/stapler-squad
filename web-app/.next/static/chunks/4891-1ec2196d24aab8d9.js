(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[4891],{

/***/ 2663:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   X: () => (/* binding */ useAbortableRequest)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ useAbortableRequest auto */ 
/**
 * Returns a `start()` function that hands back an AbortSignal for the next
 * request, automatically aborting whatever request this hook last started
 * (on the next call) and on unmount.
 *
 * A hook/component tied to a fast-changing key (session id, file path, ...)
 * that fires a request per render/effect without cancellation leaves every
 * prior request's promise — and everything it closes over — alive until it
 * resolves or times out. That's what happened in useSessionVcs: switching
 * sessions rapidly with no cancellation took the tab from ~44MB to ~154MB of
 * JS heap over 138 switches (measured 2026-09-04), each abandoned request
 * living until its own `deadline_exceeded`.
 *
 * Usage — one instance per independent request stream (a hook fetching two
 * unrelated things, like status and diff, uses two):
 *
 *   const startStatus = useAbortableRequest();
 *   const fetchStatus = useCallback(async () => {
 *     const signal = startStatus();
 *     try {
 *       const res = await client.getVCSStatus({ id }, { signal });
 *       if (signal.aborted) return;
 *       setStatus(res.vcsStatus);
 *     } catch (err) {
 *       if (signal.aborted) return;
 *       setError(err);
 *     } finally {
 *       if (!signal.aborted) setLoading(false);
 *     }
 *   }, [id, startStatus]);
 *
 * Every state update after the await must still be guarded by
 * `signal.aborted` — this hook only owns the controller's lifecycle, not
 * your component's state. The require-abort-signal ESLint rule
 * (web-app/eslint-plugin-rpc-lifecycle) enforces that every RPC client call
 * inside lib/hooks/** and lib/contexts/** passes a `{ signal }` at all.
 */ function useAbortableRequest() {
    const controllerRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const start = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        var _controllerRef_current;
        (_controllerRef_current = controllerRef.current) === null || _controllerRef_current === void 0 ? void 0 : _controllerRef_current.abort();
        const controller = new AbortController();
        controllerRef.current = controller;
        return controller.signal;
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        return ()=>{
            var _controllerRef_current;
            return (_controllerRef_current = controllerRef.current) === null || _controllerRef_current === void 0 ? void 0 : _controllerRef_current.abort();
        };
    }, []);
    return start;
}


/***/ }),

/***/ 8854:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   u: () => (/* binding */ getErrorMessage)
/* harmony export */ });
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(45640);

/**
 * Converts a thrown value from a ConnectRPC client call into a human-readable
 * message, using `ConnectError.rawMessage` (no `[code]` prefix) instead of the
 * `[code] message`-formatted `.message`. Non-ConnectError Errors fall back to
 * `.message`; anything else (including an empty `rawMessage`) falls back to
 * the caller-supplied fallback string.
 */ function getErrorMessage(e, fallback) {
    if (e instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T) return e.rawMessage || fallback;
    if (e instanceof Error) return e.message || fallback;
    return fallback;
}


/***/ }),

/***/ 16522:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  d: () => (/* binding */ SessionDetailContent)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/app/insights/SessionDetailDrawer.css.ts
var SessionDetailDrawer_css = __webpack_require__(55479);
// EXTERNAL MODULE: ./src/app/insights/insightsFormatters.ts
var insightsFormatters = __webpack_require__(97900);
;// ./src/app/insights/TokenBreakdownBar.tsx
// +feature: insights-dashboard


const SEGMENTS = [
    {
        key: "input",
        costKey: "inputCostUsd",
        label: "Input",
        color: "#4c78a8"
    },
    {
        key: "output",
        costKey: "outputCostUsd",
        label: "Output",
        color: "#f58518"
    },
    {
        key: "cacheCreation",
        costKey: "cacheCreationCostUsd",
        label: "Cache write",
        color: "#b279a2"
    },
    {
        key: "cacheRead",
        costKey: "cacheReadCostUsd",
        label: "Cache read",
        color: "#54a24b"
    }
];
/** Stacked bar of a session's four token types, with a per-category cost legend. */ function TokenBreakdownBar(props) {
    const total = SEGMENTS.reduce((sum, s)=>sum + Number(props[s.key]), 0);
    if (total === 0) return /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
        "data-testid": "token-breakdown-empty",
        children: "No token usage recorded."
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        "data-testid": "token-breakdown",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "img",
                "aria-label": "Token breakdown: ".concat(SEGMENTS.map((s)=>"".concat(s.label, " ").concat((0,insightsFormatters/* fmtTokens */.yj)(props[s.key]))).join(", ")),
                style: {
                    display: "flex",
                    height: 12,
                    borderRadius: 6,
                    overflow: "hidden"
                },
                children: SEGMENTS.map((s)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        "data-testid": "token-segment-".concat(s.key),
                        style: {
                            width: "".concat(Number(props[s.key]) / total * 100, "%"),
                            background: s.color
                        }
                    }, s.key))
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                style: {
                    display: "flex",
                    flexWrap: "wrap",
                    gap: "4px 12px",
                    listStyle: "none",
                    padding: 0,
                    margin: "6px 0 0",
                    fontSize: 12
                },
                children: SEGMENTS.map((s)=>{
                    const categoryCost = props[s.costKey];
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                        "data-testid": "token-segment-cost-".concat(s.key),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                "aria-hidden": true,
                                style: {
                                    display: "inline-block",
                                    width: 8,
                                    height: 8,
                                    background: s.color,
                                    marginRight: 4
                                }
                            }),
                            s.label,
                            ": ",
                            (0,insightsFormatters/* fmtTokens */.yj)(props[s.key]),
                            categoryCost !== undefined && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                children: [
                                    " \xb7 ",
                                    (0,insightsFormatters/* fmtCost */.F0)(categoryCost)
                                ]
                            })
                        ]
                    }, s.key);
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/ui/EstimatedValue.tsx + 1 modules
var EstimatedValue = __webpack_require__(49120);
;// ./src/app/insights/turnTimelineUtils.ts
/** A turn's total tokens must exceed this multiple of the mean to be flagged as an outlier. */ const OUTLIER_MULTIPLIER = 2;
function totalTokens(turn) {
    return Number(turn.inputTokens) + Number(turn.outputTokens) + Number(turn.cacheCreationTokens) + Number(turn.cacheReadTokens);
}
/** Sorts turns by total (input+output) tokens, descending — highest-token turn first. */ function sortTurnsByTokensDesc(turns) {
    return [
        ...turns
    ].sort((a, b)=>totalTokens(b) - totalTokens(a));
}
/** Returns OUTLIER_MULTIPLIER x the mean total-tokens-per-turn. Returns 0 for an empty list. */ function computeOutlierThreshold(turns) {
    if (turns.length === 0) return 0;
    const mean = turns.reduce((sum, t)=>sum + totalTokens(t), 0) / turns.length;
    return mean * OUTLIER_MULTIPLIER;
}
/** True when a turn's total tokens strictly exceed the outlier threshold. */ function isOutlierTurn(turn, threshold) {
    return totalTokens(turn) > threshold;
}

;// ./src/app/insights/SessionDetailContent.tsx
// +feature: insights-dashboard
/* __next_internal_client_entry_do_not_use__ SessionDetailContent auto */ 






/**
 * SessionDetailContent renders a session's Metadata, Backlog Item, Per-Turn
 * Breakdown, Tools Breakdown, and Skill Activations sections. No dialog/route
 * chrome of its own — shared verbatim by SessionDetailDrawer (modal
 * quick-peek) and the /insights/session-detail?sessionId= route's page
 * client, so the two surfaces can't drift apart (Epic 1.4, Story 1.4.2).
 */ function SessionDetailContent(param) {
    let { session, backlogEntry, turns } = param;
    const sortedTurns = (0,react.useMemo)(()=>sortTurnsByTokensDesc(turns), [
        turns
    ]);
    const outlierThreshold = (0,react.useMemo)(()=>computeOutlierThreshold(turns), [
        turns
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SessionDetailDrawer_css/* section */.uW,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: SessionDetailDrawer_css/* sectionTitle */.Gf,
                        children: "Metadata"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("dl", {
                        className: SessionDetailDrawer_css/* metaGrid */.mY,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                children: "Model"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                className: SessionDetailDrawer_css/* metaValue */.t8,
                                children: session.primaryModel || "—"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                children: "Project"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                className: SessionDetailDrawer_css/* metaValue */.t8,
                                children: session.projectPath || "—"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                children: "Total cost"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                className: SessionDetailDrawer_css/* metaValue */.t8,
                                children: (0,insightsFormatters/* fmtCost */.F0)(session.estimatedCostUsd)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                children: "Message count"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                className: SessionDetailDrawer_css/* metaValue */.t8,
                                children: session.messageCount
                            }),
                            session.sessionRole && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                        className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                        children: "Role"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                        className: SessionDetailDrawer_css/* metaValue */.t8,
                                        "data-testid": "session-role",
                                        children: session.sessionRole
                                    })
                                ]
                            }),
                            session.tags.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                        className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                        children: "Tags"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                        className: SessionDetailDrawer_css/* metaValue */.t8,
                                        children: session.tags.join(", ")
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                children: "Cache hit rate"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                className: SessionDetailDrawer_css/* metaValue */.t8,
                                children: (0,insightsFormatters/* fmtPct */.pT)(session.cacheHitRate)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                children: "Cache writes"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                className: SessionDetailDrawer_css/* metaValue */.t8,
                                children: (0,insightsFormatters/* fmtTokens */.yj)(session.cacheCreationTokens)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                children: "First message"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                className: SessionDetailDrawer_css/* metaValue */.t8,
                                children: (0,insightsFormatters/* fmtDate */.qw)(session.firstMessageAt)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                children: "Last message"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                className: SessionDetailDrawer_css/* metaValue */.t8,
                                children: (0,insightsFormatters/* fmtDate */.qw)(session.lastMessageAt)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                children: "Session ID"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                className: SessionDetailDrawer_css/* metaValue */.t8,
                                children: session.sessionId || "—"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                children: "Conversation ID"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                className: SessionDetailDrawer_css/* metaValue */.t8,
                                children: session.conversationId || "—"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SessionDetailDrawer_css/* section */.uW,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: SessionDetailDrawer_css/* sectionTitle */.Gf,
                        children: "Token Breakdown"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(TokenBreakdownBar, {
                        input: session.totalInputTokens,
                        output: session.totalOutputTokens,
                        cacheCreation: session.cacheCreationTokens,
                        cacheRead: session.cacheReadTokens,
                        inputCostUsd: session.inputCostUsd,
                        outputCostUsd: session.outputCostUsd,
                        cacheCreationCostUsd: session.cacheCreationCostUsd,
                        cacheReadCostUsd: session.cacheReadCostUsd
                    })
                ]
            }),
            backlogEntry && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SessionDetailDrawer_css/* section */.uW,
                "data-testid": "backlog-item-section",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: SessionDetailDrawer_css/* sectionTitle */.Gf,
                        children: "Backlog Item"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("dl", {
                        className: SessionDetailDrawer_css/* metaGrid */.mY,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                children: "Title"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                className: SessionDetailDrawer_css/* metaValue */.t8,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                    href: "/backlog?item=".concat(backlogEntry.itemId),
                                    className: SessionDetailDrawer_css/* backlogLink */.cX,
                                    children: backlogEntry.itemTitle
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                children: "Status"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                className: SessionDetailDrawer_css/* metaValue */.t8,
                                children: backlogEntry.itemStatus
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                                className: SessionDetailDrawer_css/* metaLabel */.Mk,
                                children: "Role"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                                className: SessionDetailDrawer_css/* metaValue */.t8,
                                children: backlogEntry.sessionRole
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SessionDetailDrawer_css/* section */.uW,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: SessionDetailDrawer_css/* sectionTitle */.Gf,
                        children: "Per-Turn Breakdown"
                    }),
                    sortedTurns.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: SessionDetailDrawer_css/* emptyState */.p$,
                        children: "No per-turn data available for this session."
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                        className: SessionDetailDrawer_css/* toolsTable */.Mh,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: SessionDetailDrawer_css/* toolsTh */.m_,
                                            children: "Timestamp"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: SessionDetailDrawer_css/* toolsTh */.m_,
                                            children: "Model"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: SessionDetailDrawer_css/* toolsThRight */.MW,
                                            children: "Input"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: SessionDetailDrawer_css/* toolsThRight */.MW,
                                            children: "Output"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: SessionDetailDrawer_css/* toolsThRight */.MW,
                                            children: "Cache"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: SessionDetailDrawer_css/* toolsTh */.m_,
                                            children: "Tools"
                                        })
                                    ]
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                                children: sortedTurns.map((t, i)=>{
                                    const outlier = isOutlierTurn(t, outlierThreshold);
                                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: SessionDetailDrawer_css/* toolsTd */.on,
                                                children: (0,insightsFormatters/* fmtDate */.qw)(t.timestamp)
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: SessionDetailDrawer_css/* toolsTd */.on,
                                                children: t.model || "—"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: SessionDetailDrawer_css/* toolsTdRight */.cu,
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: outlier ? SessionDetailDrawer_css/* outlierCell */.Lc : undefined,
                                                    children: t.inputTokens.toString()
                                                })
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: SessionDetailDrawer_css/* toolsTdRight */.cu,
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: outlier ? SessionDetailDrawer_css/* outlierCell */.Lc : undefined,
                                                    children: t.outputTokens.toString()
                                                })
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: SessionDetailDrawer_css/* toolsTdRight */.cu,
                                                title: "".concat((0,insightsFormatters/* fmtTokens */.yj)(t.cacheReadTokens), " read, ").concat((0,insightsFormatters/* fmtTokens */.yj)(t.cacheCreationTokens), " written"),
                                                children: (0,insightsFormatters/* fmtPct */.pT)((0,insightsFormatters/* computeCacheHitRate */.L0)(Number(t.inputTokens), Number(t.cacheReadTokens)))
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: SessionDetailDrawer_css/* toolsTd */.on,
                                                children: t.toolNames.join(", ") || "—"
                                            })
                                        ]
                                    }, i);
                                })
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SessionDetailDrawer_css/* section */.uW,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: SessionDetailDrawer_css/* sectionTitle */.Gf,
                        children: "Tools Breakdown"
                    }),
                    session.topTools.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: SessionDetailDrawer_css/* emptyState */.p$,
                        children: "No tools recorded for this session."
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                        className: SessionDetailDrawer_css/* toolsTable */.Mh,
                        "data-testid": "tools-breakdown-table",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: SessionDetailDrawer_css/* toolsTh */.m_,
                                            children: "Tool"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: SessionDetailDrawer_css/* toolsTh */.m_,
                                            children: "MCP Server"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: SessionDetailDrawer_css/* toolsThRight */.MW,
                                            children: "Calls"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: SessionDetailDrawer_css/* toolsThRight */.MW,
                                            children: "Cost"
                                        })
                                    ]
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                                children: session.topTools.map((t, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: SessionDetailDrawer_css/* toolsTd */.on,
                                                children: t.toolName
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: SessionDetailDrawer_css/* toolsTd */.on,
                                                children: t.mcpServer || "—"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: SessionDetailDrawer_css/* toolsTdRight */.cu,
                                                children: t.callCount
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: SessionDetailDrawer_css/* toolsTdRight */.cu,
                                                children: t.costUnpriced ? "—" : t.costMayDoubleCount ? /*#__PURE__*/ (0,jsx_runtime.jsx)(EstimatedValue/* EstimatedValue */.i, {
                                                    title: "Tool-type-level cost attribution: this tool co-occurred with another tool in the same turn, so its cost may double-count that turn's spend.",
                                                    children: (0,insightsFormatters/* fmtCost */.F0)(t.costUsd)
                                                }) : (0,insightsFormatters/* fmtCost */.F0)(t.costUsd)
                                            })
                                        ]
                                    }, i))
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SessionDetailDrawer_css/* section */.uW,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: SessionDetailDrawer_css/* sectionTitle */.Gf,
                        children: "Skill Activations"
                    }),
                    session.skillActivations.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: SessionDetailDrawer_css/* emptyState */.p$,
                        children: "No skill activations recorded."
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: SessionDetailDrawer_css/* skillList */.U8,
                        children: session.skillActivations.map((s, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: SessionDetailDrawer_css/* skillBadge */.zf,
                                children: s
                            }, i))
                    })
                ]
            })
        ]
    });
}


/***/ }),

/***/ 28765:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   FP: () => (/* binding */ timestampDate),
/* harmony export */   LI: () => (/* binding */ timestampFromDate),
/* harmony export */   R5: () => (/* binding */ timestampMs)
/* harmony export */ });
/* unused harmony exports timestampNow, timestampFromMs */
/* harmony import */ var _gen_google_protobuf_timestamp_pb_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(43963);
/* harmony import */ var _create_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(95444);
/* harmony import */ var _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(2952);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.



/**
 * Create a google.protobuf.Timestamp for the current time.
 */
function timestampNow() {
    return timestampFromDate(new Date());
}
/**
 * Create a google.protobuf.Timestamp message from an ECMAScript Date.
 */
function timestampFromDate(date) {
    return timestampFromMs(date.getTime());
}
/**
 * Convert a google.protobuf.Timestamp message to an ECMAScript Date.
 */
function timestampDate(timestamp) {
    return new Date(timestampMs(timestamp));
}
/**
 * Create a google.protobuf.Timestamp message from a Unix timestamp in milliseconds.
 */
function timestampFromMs(timestampMs) {
    const seconds = Math.floor(timestampMs / 1000);
    return (0,_create_js__WEBPACK_IMPORTED_MODULE_0__/* .create */ .v)(_gen_google_protobuf_timestamp_pb_js__WEBPACK_IMPORTED_MODULE_1__/* .TimestampSchema */ .w, {
        seconds: _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__/* .protoInt64 */ .M.parse(seconds),
        nanos: (timestampMs - seconds * 1000) * 1000000,
    });
}
/**
 * Convert a google.protobuf.Timestamp to a Unix timestamp in milliseconds.
 */
function timestampMs(timestamp) {
    return (Number(timestamp.seconds) * 1000 + Math.round(timestamp.nanos / 1000000));
}


/***/ }),

/***/ 43072:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 43963:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   i: () => (/* binding */ file_google_protobuf_timestamp),
/* harmony export */   w: () => (/* binding */ TimestampSchema)
/* harmony export */ });
/* harmony import */ var _codegenv2_file_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(1844);
/* harmony import */ var _codegenv2_message_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(15033);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


/**
 * Describes the file google/protobuf/timestamp.proto.
 */
const file_google_protobuf_timestamp = /*@__PURE__*/ (0,_codegenv2_file_js__WEBPACK_IMPORTED_MODULE_0__/* .fileDesc */ .w)("Ch9nb29nbGUvcHJvdG9idWYvdGltZXN0YW1wLnByb3RvEg9nb29nbGUucHJvdG9idWYiKwoJVGltZXN0YW1wEg8KB3NlY29uZHMYASABKAMSDQoFbmFub3MYAiABKAVChQEKE2NvbS5nb29nbGUucHJvdG9idWZCDlRpbWVzdGFtcFByb3RvUAFaMmdvb2dsZS5nb2xhbmcub3JnL3Byb3RvYnVmL3R5cGVzL2tub3duL3RpbWVzdGFtcHBi+AEBogIDR1BCqgIeR29vZ2xlLlByb3RvYnVmLldlbGxLbm93blR5cGVzYgZwcm90bzM");
/**
 * Describes the message google.protobuf.Timestamp.
 * Use `create(TimestampSchema)` to create a new message.
 */
const TimestampSchema = /*@__PURE__*/ (0,_codegenv2_message_js__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_google_protobuf_timestamp, 0);


/***/ }),

/***/ 49120:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  i: () => (/* binding */ EstimatedValue)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FEstimatedValue.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xcDRsZXgyMCB7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2YpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIGN1cnNvcjogaGVscDsKICBib3JkZXItYm90dG9tOiAxcHggZG90dGVkIHZhcigtLV8xOWV6MnJ6ZSk7Cn0KLl8xcDRsZXgyMSB7CiAgcG9zaXRpb246IGFic29sdXRlOwogIHdpZHRoOiAxcHg7CiAgaGVpZ2h0OiAxcHg7CiAgb3ZlcmZsb3c6IGhpZGRlbjsKICBjbGlwOiByZWN0KDAsMCwwLDApOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FEstimatedValue_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xcDRsZXgyMCB7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2YpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIGN1cnNvcjogaGVscDsKICBib3JkZXItYm90dG9tOiAxcHggZG90dGVkIHZhcigtLV8xOWV6MnJ6ZSk7Cn0KLl8xcDRsZXgyMSB7CiAgcG9zaXRpb246IGFic29sdXRlOwogIHdpZHRoOiAxcHg7CiAgaGVpZ2h0OiAxcHg7CiAgb3ZlcmZsb3c6IGhpZGRlbjsKICBjbGlwOiByZWN0KDAsMCwwLDApOwp9_22_7D = __webpack_require__(60271);
;// ./src/components/ui/EstimatedValue.css.ts

var estimatedValueMarker = '_1p4lex20';
var srOnly = '_1p4lex21';
;// ./src/components/ui/EstimatedValue.tsx
// +feature: estimated-value
/* __next_internal_client_entry_do_not_use__ EstimatedValue auto */ 


// EstimatedValue is the one shared visual treatment for any modeled/heuristic
// number on the insights dashboard (per-tool cost, activity cost, cache ROI,
// waste score) — reused rather than forked across those surfaces per
// research/pitfalls.md §2 and research/ux.md §4. Renders "~{children}" with
// an aria-describedby pointing at hidden text explaining the estimation
// method, so screen-reader users get the same caveat sighted users see via
// the title tooltip.
function EstimatedValue(param) {
    let { children, title, className } = param;
    const descriptionId = (0,react.useId)();
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: className ? "".concat(estimatedValueMarker, " ").concat(className) : estimatedValueMarker,
                title: title,
                "aria-describedby": descriptionId,
                children: [
                    "~",
                    children
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                id: descriptionId,
                className: srOnly,
                children: title
            })
        ]
    });
}


/***/ }),

/***/ 55479:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Gf: () => (/* binding */ sectionTitle),
/* harmony export */   HQ: () => (/* binding */ openFullPageLink),
/* harmony export */   Lc: () => (/* binding */ outlierCell),
/* harmony export */   Lw: () => (/* binding */ overlay),
/* harmony export */   MW: () => (/* binding */ toolsThRight),
/* harmony export */   Mh: () => (/* binding */ toolsTable),
/* harmony export */   Mk: () => (/* binding */ metaLabel),
/* harmony export */   U8: () => (/* binding */ skillList),
/* harmony export */   b: () => (/* binding */ closeButton),
/* harmony export */   cG: () => (/* binding */ drawerTitle),
/* harmony export */   cX: () => (/* binding */ backlogLink),
/* harmony export */   cu: () => (/* binding */ toolsTdRight),
/* harmony export */   fF: () => (/* binding */ drawerHeader),
/* harmony export */   mY: () => (/* binding */ metaGrid),
/* harmony export */   m_: () => (/* binding */ toolsTh),
/* harmony export */   on: () => (/* binding */ toolsTd),
/* harmony export */   p$: () => (/* binding */ emptyState),
/* harmony export */   qO: () => (/* binding */ sessionIdChip),
/* harmony export */   sM: () => (/* binding */ srOnly),
/* harmony export */   t8: () => (/* binding */ metaValue),
/* harmony export */   uW: () => (/* binding */ section),
/* harmony export */   w$: () => (/* binding */ drawer),
/* harmony export */   zf: () => (/* binding */ skillBadge)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fapp_2Finsights_2FSessionDetailDrawer_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VX247jKBB976_gZaVEalp2pqe7w7zsZ_zbCkPZpoPBAty5rObfV4CT4HhsJ3tRIhmbcl1OVZ3Cv_gWBragEX167ukWYb_ekKoNLoJC4ScocqW2jQkLiV18Mcqz7Lf1j_eEPr5hJDTs7JZL_jz6SXayIN4q61wQiuCSnEA7mWEsuAIyvy6oGxXGd0pTpCpCrrKnsPv5TWoO2GhOBwIettuf1xVb6ZUO932io2o6rORGuJNnmVftX_wF9zVBDVCrV4_svbwjLbZ1z5Y1F9gSqn3_EgQ7Zy_dfKLmhXGf_ZbOG3MaRveKbThYAjK2wOyWgp_I_WZSOHeMaUVDCJ8z4KvVImGxqjOqcpeNhYBtYB157wMF7aV9EhQKeHgH_gr5sIAi28yLbtG_Z2KRkAu0H0L0I00UCkqhYWDxhLEQDkw_vFnZ50oj5hp5UA5gmxLGeAC3B4gGGgp50JVt8B8y9fjJwkKhXZON9OQletLXLY2Qu2GUbw_FEUAYah_c4oGtHLYihOM_Ifr_r4vnxuJOkgwLbW53fO9cPH1e_w1r6mkjZDHkS_dgi902tJmPV_gbD1Ikof7l0Cci5Ny0dmRA293lXmZRv327_rsutX36uajPaS9fL7_rxwfghh4raUGlFuAnnXG_r1Wi7PH04lk81X9RmpPPwG56ZR_3Fl375Ek_8_fJSIbiwO3SWSWodZLSSPCAw9iYR5Ef_4dtVMz_zD_o2pdXBwOBl2XduCYdQG3pbgHBjs_TDAm71k36HxOw01lVDDzG6H3VAZEcaVv2IHTetHKI5g2Th3UF5OctdxlLs4fAfNVVALUgwwo3dhNoPIvhYOQszgE7I3tE3UFwvqi_nCRWivDceFAbojKFwwlTIxwIKBnhD82SQpWKalpK2FUHNhNd_KRVp7seBCvgOGBEko3cx4G6dgcaAsk_hDk3GmsKs0NBiFFg4giUQ5xxOb00SgM2l8PJBJwC5OVoth1Au0eZx2_4ReEEpGxdE4ov49fAL7OibhqDYOlPtnaj6HKoSyvc0LqRmu4FDm8nJPolI_j4NSX4HJNu7Kv6hU8MuRDvTJEvsEvfdUQJBwlEpWKJd3nw20MJq2TlIjhn58JTR317TnhI87nfPnwwE1YLzeCxmUrQE_Tm4yp5R_5_m0CtM6dRo7uLrCYWhVzgwbfrPiZsBqqahbtYzCpZzsETjKjnmjIx0ioPp59bfUw4zuLcOAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(43072);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fapp_2Finsights_2FSessionDetailDrawer_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VX247jKBB976_gZaVEalp2pqe7w7zsZ_zbCkPZpoPBAty5rObfV4CT4HhsJ3tRIhmbcl1OVZ3Cv_gWBragEX167ukWYb_ekKoNLoJC4ScocqW2jQkLiV18Mcqz7Lf1j_eEPr5hJDTs7JZL_jz6SXayIN4q61wQiuCSnEA7mWEsuAIyvy6oGxXGd0pTpCpCrrKnsPv5TWoO2GhOBwIettuf1xVb6ZUO932io2o6rORGuJNnmVftX_wF9zVBDVCrV4_svbwjLbZ1z5Y1F9gSqn3_EgQ7Zy_dfKLmhXGf_ZbOG3MaRveKbThYAjK2wOyWgp_I_WZSOHeMaUVDCJ8z4KvVImGxqjOqcpeNhYBtYB157wMF7aV9EhQKeHgH_gr5sIAi28yLbtG_Z2KRkAu0H0L0I00UCkqhYWDxhLEQDkw_vFnZ50oj5hp5UA5gmxLGeAC3B4gGGgp50JVt8B8y9fjJwkKhXZON9OQletLXLY2Qu2GUbw_FEUAYah_c4oGtHLYihOM_Ifr_r4vnxuJOkgwLbW53fO9cPH1e_w1r6mkjZDHkS_dgi902tJmPV_gbD1Ikof7l0Cci5Ny0dmRA293lXmZRv327_rsutX36uajPaS9fL7_rxwfghh4raUGlFuAnnXG_r1Wi7PH04lk81X9RmpPPwG56ZR_3Fl375Ek_8_fJSIbiwO3SWSWodZLSSPCAw9iYR5Ef_4dtVMz_zD_o2pdXBwOBl2XduCYdQG3pbgHBjs_TDAm71k36HxOw01lVDDzG6H3VAZEcaVv2IHTetHKI5g2Th3UF5OctdxlLs4fAfNVVALUgwwo3dhNoPIvhYOQszgE7I3tE3UFwvqi_nCRWivDceFAbojKFwwlTIxwIKBnhD82SQpWKalpK2FUHNhNd_KRVp7seBCvgOGBEko3cx4G6dgcaAsk_hDk3GmsKs0NBiFFg4giUQ5xxOb00SgM2l8PJBJwC5OVoth1Au0eZx2_4ReEEpGxdE4ov49fAL7OibhqDYOlPtnaj6HKoSyvc0LqRmu4FDm8nJPolI_j4NSX4HJNu7Kv6hU8MuRDvTJEvsEvfdUQJBwlEpWKJd3nw20MJq2TlIjhn58JTR317TnhI87nfPnwwE1YLzeCxmUrQE_Tm4yp5R_5_m0CtM6dRo7uLrCYWhVzgwbfrPiZsBqqahbtYzCpZzsETjKjnmjIx0ioPp59bfUw4zuLcOAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fapp_2Finsights_2FSessionDetailDrawer_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VX247jKBB976_gZaVEalp2pqe7w7zsZ_zbCkPZpoPBAty5rObfV4CT4HhsJ3tRIhmbcl1OVZ3Cv_gWBragEX167ukWYb_ekKoNLoJC4ScocqW2jQkLiV18Mcqz7Lf1j_eEPr5hJDTs7JZL_jz6SXayIN4q61wQiuCSnEA7mWEsuAIyvy6oGxXGd0pTpCpCrrKnsPv5TWoO2GhOBwIettuf1xVb6ZUO932io2o6rORGuJNnmVftX_wF9zVBDVCrV4_svbwjLbZ1z5Y1F9gSqn3_EgQ7Zy_dfKLmhXGf_ZbOG3MaRveKbThYAjK2wOyWgp_I_WZSOHeMaUVDCJ8z4KvVImGxqjOqcpeNhYBtYB157wMF7aV9EhQKeHgH_gr5sIAi28yLbtG_Z2KRkAu0H0L0I00UCkqhYWDxhLEQDkw_vFnZ50oj5hp5UA5gmxLGeAC3B4gGGgp50JVt8B8y9fjJwkKhXZON9OQletLXLY2Qu2GUbw_FEUAYah_c4oGtHLYihOM_Ifr_r4vnxuJOkgwLbW53fO9cPH1e_w1r6mkjZDHkS_dgi902tJmPV_gbD1Ikof7l0Cci5Ny0dmRA293lXmZRv327_rsutX36uajPaS9fL7_rxwfghh4raUGlFuAnnXG_r1Wi7PH04lk81X9RmpPPwG56ZR_3Fl375Ek_8_fJSIbiwO3SWSWodZLSSPCAw9iYR5Ef_4dtVMz_zD_o2pdXBwOBl2XduCYdQG3pbgHBjs_TDAm71k36HxOw01lVDDzG6H3VAZEcaVv2IHTetHKI5g2Th3UF5OctdxlLs4fAfNVVALUgwwo3dhNoPIvhYOQszgE7I3tE3UFwvqi_nCRWivDceFAbojKFwwlTIxwIKBnhD82SQpWKalpK2FUHNhNd_KRVp7seBCvgOGBEko3cx4G6dgcaAsk_hDk3GmsKs0NBiFFg4giUQ5xxOb00SgM2l8PJBJwC5OVoth1Au0eZx2_4ReEEpGxdE4ov49fAL7OibhqDYOlPtnaj6HKoSyvc0LqRmu4FDm8nJPolI_j4NSX4HJNu7Kv6hU8MuRDvTJEvsEvfdUQJBwlEpWKJd3nw20MJq2TlIjhn58JTR317TnhI87nfPnwwE1YLzeCxmUrQE_Tm4yp5R_5_m0CtM6dRo7uLrCYWhVzgwbfrPiZsBqqahbtYzCpZzsETjKjnmjIx0ioPp59bfUw4zuLcOAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);

var backlogLink = 'h47la0n';
var closeButton = 'h47la06';
var drawer = 'h47la02';
var drawerHeader = 'h47la03';
var drawerTitle = 'h47la04';
var emptyState = 'h47la0k';
var metaGrid = 'h47la09';
var metaLabel = 'h47la0a';
var metaValue = 'h47la0b';
var openFullPageLink = 'h47la0m';
var outlierCell = 'h47la0h';
var overlay = 'h47la01';
var section = 'h47la07';
var sectionTitle = 'h47la08';
var sessionIdChip = 'h47la05';
var skillBadge = 'h47la0j';
var skillList = 'h47la0i';
var srOnly = 'h47la0l';
var toolsTable = 'h47la0c';
var toolsTd = 'h47la0f';
var toolsTdRight = 'h47la0g h47la0f';
var toolsTh = 'h47la0d';
var toolsThRight = 'h47la0e h47la0d';

/***/ }),

/***/ 60271:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 64343:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   B: () => (/* binding */ useAbortableEffect)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(2663);
/* __next_internal_client_entry_do_not_use__ useAbortableEffect auto */ 

/**
 * Runs `effect(signal)` on mount and whenever `deps` change, cancelling the
 * previous run first (and on unmount) via useAbortableRequest — the common
 * "declare an AbortController ref, wrap the fetch in useCallback, call it
 * from useEffect" boilerplate collapsed into one call.
 *
 * `effect` still must check `signal.aborted` before every state update
 * after an `await` — this hook only owns cancellation timing, not your
 * component's state (same contract as useAbortableRequest). It intentionally
 * does not manage data/loading/error state itself: hooks in this codebase
 * differ too much in error handling (NotFound stops polling, some errors
 * are non-fatal, some fall back to a cached value) for one generic shape to
 * fit without fighting those differences — see useAbortableRequest's doc
 * comment for the lower-level primitive this builds on.
 *
 * Usage:
 *
 *   useAbortableEffect(async (signal) => {
 *     setLoading(true);
 *     try {
 *       const res = await client.getThing({ id }, { signal });
 *       if (signal.aborted) return;
 *       setData(res.thing);
 *     } catch (err) {
 *       if (signal.aborted) return;
 *       setError(err);
 *     } finally {
 *       if (!signal.aborted) setLoading(false);
 *     }
 *   }, [id]);
 */ function useAbortableEffect(effect, deps) {
    const start = (0,_lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_1__/* .useAbortableRequest */ .X)();
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const signal = start();
        // effect() is documented to handle its own errors; this catch only
        // guards against a caller that doesn't, so a stray rejection doesn't
        // surface as an unhandled promise rejection.
        void effect(signal).catch(()=>{});
    // effect and start are intentionally excluded: callers list their own
    // fetch dependencies in `deps`, same convention as every hook this
    // replaces (e.g. useSessionVcs.ts's fetchStatus effect).
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, deps);
}


/***/ }),

/***/ 97900:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   F0: () => (/* binding */ fmtCost),
/* harmony export */   L0: () => (/* binding */ computeCacheHitRate),
/* harmony export */   jd: () => (/* binding */ roleDisplayLabel),
/* harmony export */   m_: () => (/* binding */ shortId),
/* harmony export */   pT: () => (/* binding */ fmtPct),
/* harmony export */   qw: () => (/* binding */ fmtDate),
/* harmony export */   uk: () => (/* binding */ pathBasename),
/* harmony export */   yj: () => (/* binding */ fmtTokens)
/* harmony export */ });
// +feature: insights-dashboard
// Shared formatting helpers for all insights components.
/** Format a USD cost with adaptive decimal precision. */ function fmtCost(usd) {
    if (usd < 0.01) return "$".concat(usd.toFixed(4));
    if (usd < 1) return "$".concat(usd.toFixed(3));
    return "$".concat(usd.toFixed(2));
}
/** Format a token count with M/K abbreviations. */ function fmtTokens(n) {
    const num = Number(n);
    if (num >= 1000000) return "".concat((num / 1000000).toFixed(1), "M");
    if (num >= 1000) return "".concat((num / 1000).toFixed(1), "K");
    return num.toString();
}
/** Format a cache-hit rate (0–1) as a percentage with 1 decimal place. */ function fmtPct(rate) {
    return "".concat((rate * 100).toFixed(1), "%");
}
/** Cache-hit rate: cacheRead / (input + cacheRead), 0-guarded. */ function computeCacheHitRate(input, cacheRead) {
    const denom = input + cacheRead;
    return denom === 0 ? 0 : cacheRead / denom;
}
/** Format a protobuf Timestamp as a short human-readable date. */ function fmtDate(ts) {
    if (!ts) return "—";
    return new Date(Number(ts.seconds) * 1000).toLocaleDateString(undefined, {
        month: "short",
        day: "numeric",
        year: "numeric"
    });
}
/** Return the first 8 characters of an ID followed by an ellipsis. */ function shortId(id) {
    return id.length > 8 ? id.slice(0, 8) + "…" : id;
}
/**
 * Display text for a SessionRole/session_role value ("" for a stapler-squad
 * session with no backlog attribution). The raw "" must stay "" everywhere it's
 * used as data (filter state, StageCostChart's dataKey, SessionTokenSummary
 * comparisons) — this is for text shown to a person only.
 */ function roleDisplayLabel(role) {
    return role || "unattributed";
}
// Session worktree dirs are `<title>_<16-hex id>`; the transcript's project path
// decoding turns every "_" and "-" into "/", so the id is the segment after the title.
const WORKTREE_PATH = /\/worktrees\/(.+?)\/[0-9a-f]{16}(?:\/|$)/;
/**
 * Return a project path's display name: the session title for a stapler-squad
 * worktree path (whose last segment is just the opaque id), else its final path
 * segment. The title is approximate: original "-", "_" and "." are indistinguishable.
 */ function pathBasename(p) {
    var _WORKTREE_PATH_exec;
    const title = (_WORKTREE_PATH_exec = WORKTREE_PATH.exec(p)) === null || _WORKTREE_PATH_exec === void 0 ? void 0 : _WORKTREE_PATH_exec[1];
    return title ? title.replace(/\//g, "-") : p.split("/").pop() || p;
}


/***/ })

}]);
//# sourceMappingURL=4891-1ec2196d24aab8d9.js.map