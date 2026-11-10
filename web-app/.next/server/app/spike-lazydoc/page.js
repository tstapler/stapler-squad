(() => {
var exports = {};
exports.id = 4914;
exports.ids = [4914];
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

/***/ 17439:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ SpikeLazyDocPage)
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
;// ./src/lib/__tests__/virtualFile.ts
// Deterministic fake file source for the lazy-document spike page and the Story 1.6.4 tests (moved here from
// app/spike-lazydoc, Task 1.6.4a, so tests need not import from app/). No storage: each line is
// regenerated from (seed, index), so a 1,000,000-line / ~50 MB file costs nothing until fetched.
const VIRTUAL_LINE_COUNT = 1000000;
const FILLER = "abcdefghijklmnopqrstuvwxyz0123456789 _-=+(){}[];:,.";
function hash32(n) {
    let h = (n ^ 0x9e3779b9) >>> 0;
    h = Math.imul(h ^ h >>> 16, 0x85ebca6b);
    h = Math.imul(h ^ h >>> 13, 0xc2b2ae35);
    return (h ^ h >>> 16) >>> 0;
}
/** Line text for 0-based index `i`: "L<i+1> " plus 14..74 filler chars, ~50 bytes on average. */ function lineText(i, seed = 1) {
    const h = hash32(i * 2654435761 + seed);
    const len = 14 + h % 61;
    let out = `L${i + 1} `;
    let x = h;
    for(let k = 0; k < len; k++){
        x = hash32(x + k);
        out += FILLER[x % FILLER.length];
    }
    return out;
}
class VirtualDoc {
    constructor(lineCount = VIRTUAL_LINE_COUNT, seed = 1){
        this.lineCount = lineCount;
        this.seed = seed;
    }
    fetch(start, count) {
        const end = Math.min(this.lineCount, start + count);
        const out = [];
        for(let i = start; i < end; i++)out.push(lineText(i, this.seed));
        return out;
    }
}
/** mulberry32: small seeded PRNG so every spike and oracle run is reproducible. */ function prng(seed) {
    let a = seed >>> 0;
    return ()=>{
        a = a + 0x6d2b79f5 >>> 0;
        let t = a;
        t = Math.imul(t ^ t >>> 15, t | 1);
        t ^= t + Math.imul(t ^ t >>> 7, t | 61);
        return ((t ^ t >>> 14) >>> 0) / 4294967296;
    };
}

;// ./src/app/spike-lazydoc/gapLine.ts
// THROWAWAY (Task 0.1.4c, ADR-036 S-T3) prototype a2: gap-line model. Loaded lines plus one block-replace
// placeholder line per unloaded gap; the gap table lives in a StateField; load/evict are non-history
// transactions; changeFilter and atomicRanges protect the placeholders. Promoted to cm6LazyDoc in Story 1.6.4.


const LINE_HEIGHT = 20;
const CHUNK_LINES = 500;
/** One placeholder never spans more than this many lines: a 250k-line (5M px) block defeated CM6's height scaler (jump landed at line 247,375 instead of 500,000). */ const MAX_GAP_LINES = 5000;
const PLACEHOLDER = "​";
const setSegs = dist/* StateEffect */.Pe.define();
const lazyLoad = dist/* Annotation */.YH.define();
function withPrefix(segs) {
    const prefix = [];
    let acc = 0;
    for (const s of segs){
        prefix.push(acc);
        acc += s.lines;
    }
    return {
        segs,
        prefix
    };
}
function remap(table, tr) {
    const doc = tr.newDoc;
    const segs = table.segs.map((s)=>{
        const from = tr.changes.mapPos(s.from, -1);
        const to = tr.changes.mapPos(s.to, 1);
        let touched = false;
        tr.changes.iterChangedRanges((fa, ta)=>{
            if (s.kind === "loaded" && fa <= s.to && ta >= s.from) touched = true;
        });
        const lines = s.kind === "gap" ? s.lines : doc.lineAt(to).number - doc.lineAt(from).number + 1;
        return {
            ...s,
            from,
            to,
            lines,
            dirty: s.dirty || touched
        };
    });
    return withPrefix(segs);
}
function createGapTableField(initial) {
    return dist/* StateField */.sU.define({
        create: ()=>withPrefix(initial),
        update (table, tr) {
            for (const e of tr.effects)if (e.is(setSegs)) return withPrefix(e.value);
            return tr.docChanged ? remap(table, tr) : table;
        }
    });
}
let tableField;
const gapTable = (state)=>state.field(tableField);
function segIndexAtPos(table, pos) {
    let lo = 0;
    let hi = table.segs.length - 1;
    while(lo < hi){
        const mid = lo + hi + 1 >> 1;
        if (table.segs[mid].from <= pos) lo = mid;
        else hi = mid - 1;
    }
    return lo;
}
/** 1-based file line number shown in the gutter for a 1-based document line. */ function absLineOfDocLine(state, docLine) {
    // The gutter may format a line number from a document one step behind the state it is handed.
    if (docLine > state.doc.lines) return docLine;
    const table = gapTable(state);
    const line = state.doc.line(docLine);
    const i = segIndexAtPos(table, line.from);
    const seg = table.segs[i];
    if (seg.kind === "gap") return table.prefix[i] + 1;
    return table.prefix[i] + (docLine - state.doc.lineAt(seg.from).number) + 1;
}
/** Converts a document position to 0-based file line and UTF-16 column (the oracle's coordinates). */ function docPosToAbs(state, pos) {
    const dl = state.doc.lineAt(pos);
    return {
        line: absLineOfDocLine(state, dl.number) - 1,
        col: pos - dl.from
    };
}
class GapWidget extends view_dist/* WidgetType */.xO {
    constructor(lines){
        super(), this.lines = lines;
    }
    eq(o) {
        return o.lines === this.lines;
    }
    get estimatedHeight() {
        return this.lines * LINE_HEIGHT;
    }
    toDOM() {
        const el = document.createElement("div");
        el.className = "cm-lazy-gap";
        el.setAttribute("aria-hidden", "true");
        el.style.height = `${this.lines * LINE_HEIGHT}px`;
        el.textContent = `… ${this.lines} lines not loaded`;
        return el;
    }
    ignoreEvent() {
        return true;
    }
}
function placeholders(state) {
    const ranges = gapTable(state).segs.filter((s)=>s.kind === "gap").map((s)=>view_dist/* Decoration */.NZ.replace({
            widget: new GapWidget(s.lines),
            block: true
        }).range(s.from, s.to));
    return view_dist/* Decoration */.NZ.set(ranges, true);
}
const placeholderField = dist/* StateField */.sU.define({
    create: placeholders,
    update: (_v, tr)=>placeholders(tr.state),
    provide: (f)=>[
            view_dist/* EditorView */.Lz.decorations.from(f),
            view_dist/* EditorView */.Lz.atomicRanges.of((v)=>v.state.field(f))
        ]
});
function touchesPlaceholder(table, tr) {
    let bad = false;
    tr.changes.iterChangedRanges((fa, ta)=>{
        for (const s of table.segs){
            if (s.kind !== "gap") continue;
            const ps = Math.max(0, s.from - 1);
            const pe = s.to + 1;
            const hit = ta > fa ? fa < pe && ta > ps : fa > ps && fa < pe;
            if (hit) bad = true;
        }
    });
    return bad;
}
function gapLineExtension(vdoc, initial) {
    const segs = initial ?? splitGap(0, vdoc.lineCount);
    tableField = createGapTableField(segs);
    const initialDoc = segs.map((s)=>PLACEHOLDER).join("\n");
    const extension = [
        tableField,
        placeholderField,
        (0,view_dist/* lineNumbers */.$K)({
            formatNumber: (n, state)=>String(absLineOfDocLine(state, n))
        }),
        dist/* EditorState */.$t.changeFilter.of((tr)=>tr.annotation(lazyLoad) || !touchesPlaceholder(gapTable(tr.startState), tr)),
        view_dist/* EditorView */.Lz.theme({
            ".cm-line": {
                height: `${LINE_HEIGHT}px`,
                lineHeight: `${LINE_HEIGHT}px`
            }
        })
    ];
    return {
        extension,
        initialDoc
    };
}
/** Initial gap pieces covering [start, start + total), each at most MAX_GAP_LINES, with doc offsets. */ function splitGap(start, total) {
    const segs = [];
    let pos = 0;
    for(let done = 0; done < total; done += MAX_GAP_LINES){
        const lines = Math.min(MAX_GAP_LINES, total - done);
        segs.push({
            kind: "gap",
            from: pos,
            to: pos + 1,
            lines,
            baseStart: start + done,
            dirty: false
        });
        pos += 2;
    }
    return segs;
}
function shift(segs, delta) {
    return segs.map((s)=>({
            ...s,
            from: s.from + delta,
            to: s.to + delta
        }));
}
/** Loads the CHUNK_LINES-aligned chunk containing file line `abs` (0-based). Returns false if nothing to do or deferred. */ function loadChunk(view, vdoc, abs) {
    if (view.composing) return false;
    const table = gapTable(view.state);
    const i = table.segs.findIndex((s, k)=>s.kind === "gap" && abs >= table.prefix[k] && abs < table.prefix[k] + s.lines);
    if (i < 0) return false;
    const g = table.segs[i];
    const gs = table.prefix[i];
    const s = Math.max(gs, Math.floor(abs / CHUNK_LINES) * CHUNK_LINES);
    const e = Math.min(gs + g.lines, s + CHUNK_LINES);
    const loadedText = vdoc.fetch(g.baseStart + (s - gs), e - s).join("\n");
    const pieces = [];
    if (s > gs) pieces.push({
        text: PLACEHOLDER,
        seg: {
            kind: "gap",
            lines: s - gs,
            baseStart: g.baseStart,
            dirty: false
        }
    });
    pieces.push({
        text: loadedText,
        seg: {
            kind: "loaded",
            lines: e - s,
            baseStart: g.baseStart + (s - gs),
            dirty: false
        }
    });
    if (e < gs + g.lines) pieces.push({
        text: PLACEHOLDER,
        seg: {
            kind: "gap",
            lines: gs + g.lines - e,
            baseStart: g.baseStart + (e - gs),
            dirty: false
        }
    });
    const insert = pieces.map((p)=>p.text).join("\n");
    let pos = g.from;
    const fresh = pieces.map((p)=>{
        const seg = {
            ...p.seg,
            from: pos,
            to: pos + p.text.length
        };
        pos += p.text.length + 1;
        return seg;
    });
    const delta = insert.length - (g.to - g.from);
    const next = [
        ...table.segs.slice(0, i),
        ...fresh,
        ...shift(table.segs.slice(i + 1), delta)
    ];
    view.dispatch({
        changes: {
            from: g.from,
            to: g.to,
            insert
        },
        effects: setSegs.of(next),
        annotations: [
            dist/* Transaction */.ZX.addToHistory.of(false),
            lazyLoad.of(true)
        ]
    });
    return true;
}
/** Turns clean loaded segment `i` back into a placeholder, merging with adjacent gaps while under the cap. */ function evictSegment(view, i) {
    if (view.composing) return false;
    const table = gapTable(view.state);
    const seg = table.segs[i];
    if (!seg || seg.kind !== "loaded" || seg.dirty) return false;
    let lo = i;
    let hi = i;
    let lines = seg.lines;
    const prev = table.segs[i - 1];
    if (prev?.kind === "gap" && prev.baseStart + prev.lines === seg.baseStart && lines + prev.lines <= MAX_GAP_LINES) {
        lo = i - 1;
        lines += prev.lines;
    }
    const nextSeg = table.segs[i + 1];
    if (nextSeg?.kind === "gap" && seg.baseStart + seg.lines === nextSeg.baseStart && lines + nextSeg.lines <= MAX_GAP_LINES) {
        hi = i + 1;
        lines += nextSeg.lines;
    }
    const from = table.segs[lo].from;
    const to = table.segs[hi].to;
    const merged = {
        kind: "gap",
        from,
        to: from + 1,
        lines,
        baseStart: table.segs[lo].baseStart,
        dirty: false
    };
    const delta = merged.to - to;
    const next = [
        ...table.segs.slice(0, lo),
        merged,
        ...shift(table.segs.slice(hi + 1), delta)
    ];
    view.dispatch({
        changes: {
            from,
            to,
            insert: PLACEHOLDER
        },
        effects: setSegs.of(next),
        annotations: [
            dist/* Transaction */.ZX.addToHistory.of(false),
            lazyLoad.of(true)
        ]
    });
    return true;
}
/** Current file as lines: loaded text from the document, gaps regenerated from the source. */ function materialize(state, vdoc) {
    const out = [];
    const doc = state.doc;
    for (const s of gapTable(state).segs){
        const lines = s.kind === "gap" ? vdoc.fetch(s.baseStart, s.lines) : doc.sliceString(s.from, s.to).split("\n");
        for (const l of lines)out.push(l);
    }
    return out;
}
function loadedChars(state) {
    return gapTable(state).segs.reduce((n, s)=>s.kind === "loaded" ? n + (s.to - s.from) : n, 0);
}

;// ./src/app/spike-lazydoc/spikeApi.ts
// THROWAWAY (Task 0.1.4c): the surface the Playwright spec and the owner page read through window.__spike.api.
function spikeApi(api) {
    window.__spike = {
        api
    };
}
/** Client top of the first rendered line that intersects the scroller: a DOM fact, not a CodeMirror estimate. */ function domTopLineClientTop(view) {
    const r = view.scrollDOM.getBoundingClientRect();
    for (const el of Array.from(view.scrollDOM.querySelectorAll(".cm-line"))){
        const b = el.getBoundingClientRect();
        if (b.bottom > r.top) return b.top;
    }
    return NaN;
}

;// ./src/app/spike-lazydoc/A2Host.tsx
/* __next_internal_client_entry_do_not_use__ A2Host auto */ 
// THROWAWAY (Task 0.1.4c): mounts prototype a2 (gap-line) over the 1,000,000-line fake file.







const BUDGET_CHARS = 8 * 1024 * 1024;
function topAbs(view) {
    const h = view.scrollDOM.getBoundingClientRect().top - view.documentTop;
    const block = view.lineBlockAtHeight(h);
    const table = gapTable(view.state);
    const i = table.segs.findIndex((s)=>s.from <= block.from && block.from <= s.to);
    const seg = table.segs[i];
    if (seg.kind === "loaded") return absLineOfDocLine(view.state, view.state.doc.lineAt(block.from).number);
    return -1;
}
/** Loads the gap under the viewport top and bottom, then evicts the least recently seen clean chunks over budget. */ function autoLoad(view, vdoc, seen) {
    let loaded = false;
    const rect = view.scrollDOM.getBoundingClientRect();
    const top = rect.top - view.documentTop;
    for (const h of [
        top,
        top + rect.height - 1
    ]){
        const block = view.lineBlockAtHeight(h);
        const table = gapTable(view.state);
        const i = table.segs.findIndex((s)=>s.kind === "gap" && s.from <= block.from && block.from <= s.to);
        if (i >= 0 && loadChunk(view, vdoc, table.prefix[i] + Math.max(0, Math.floor((h - block.top) / LINE_HEIGHT)))) loaded = true;
    }
    const table = gapTable(view.state);
    const vp = view.viewport;
    table.segs.forEach((s)=>{
        if (s.kind === "loaded" && s.from <= vp.to && s.to >= vp.from) seen.set(s.baseStart, performance.now());
    });
    while(loadedChars(view.state) > BUDGET_CHARS){
        const t = gapTable(view.state);
        const victims = t.segs.map((s, i)=>({
                s,
                i
            })).filter(({ s })=>s.kind === "loaded" && !s.dirty && !(s.from <= vp.to && s.to >= vp.from));
        if (!victims.length) return loaded;
        victims.sort((a, b)=>(seen.get(a.s.baseStart) ?? 0) - (seen.get(b.s.baseStart) ?? 0));
        if (!evictSegment(view, victims[0].i)) return loaded;
    }
    return loaded;
}
function A2Host({ onStats }) {
    const host = (0,react.useRef)(null);
    const [ready, setReady] = (0,react.useState)(false);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!host.current) return;
        const vdoc = new VirtualDoc();
        const { extension, initialDoc } = gapLineExtension(vdoc, splitGap(0, vdoc.lineCount));
        const view = new view_dist/* EditorView */.Lz({
            state: dist/* EditorState */.$t.create({
                doc: initialDoc,
                extensions: [
                    (0,commands_dist/* history */.b6)(),
                    extension,
                    view_dist/* EditorView */.Lz.theme({
                        "&": {
                            height: "100%"
                        }
                    })
                ]
            }),
            parent: host.current
        });
        const seen = new Map();
        let raf = 0;
        let settleRuns = 0;
        const tick = ()=>{
            raf = 0;
            // A load changes layout without a scroll event (WebKit), so keep going until a pass loads nothing.
            if (!view.composing && autoLoad(view, vdoc, seen) && settleRuns++ < 20) schedule();
            else settleRuns = 0;
            onStats(`a2 top=${topAbs(view)} loaded=${(loadedChars(view.state) / 1048576).toFixed(2)}MiB segs=${gapTable(view.state).segs.length} undo=${(0,commands_dist/* undoDepth */.mk)(view.state)}`);
        };
        const schedule = ()=>{
            if (!raf) raf = requestAnimationFrame(tick);
        };
        view.scrollDOM.addEventListener("scroll", schedule, {
            passive: true
        });
        loadChunk(view, vdoc, 0);
        schedule();
        const api = {
            lineHeight: ()=>LINE_HEIGHT,
            topAbs: ()=>topAbs(view),
            topLineClientTop: ()=>domTopLineClientTop(view),
            scrollToAbs (n) {
                loadChunk(view, vdoc, n - 1);
                const t = gapTable(view.state);
                const i = t.segs.findIndex((s, k)=>s.kind === "loaded" && n - 1 >= t.prefix[k] && n - 1 < t.prefix[k] + s.lines);
                const seg = t.segs[i];
                const line = view.state.doc.line(view.state.doc.lineAt(seg.from).number + (n - 1 - t.prefix[i]));
                view.dispatch({
                    effects: view_dist/* EditorView */.Lz.scrollIntoView(line.from, {
                        y: "start"
                    })
                });
            },
            loadAboveViewport (linesAbove) {
                const before = {
                    top: topAbs(view),
                    y: domTopLineClientTop(view)
                };
                const ok = loadChunk(view, vdoc, Math.max(0, before.top - linesAbove));
                return {
                    ok,
                    before
                };
            },
            cycles (n) {
                const start = Math.max(topAbs(view), 1000);
                for(let c = 0; c < n; c++){
                    const target = start + 20000 + c % 40 * 500;
                    loadChunk(view, vdoc, target);
                    const t = gapTable(view.state);
                    const idx = t.segs.findIndex((s, i)=>s.kind === "loaded" && !s.dirty && t.prefix[i] === target - target % 500);
                    if (idx >= 0) evictSegment(view, idx);
                }
            },
            type (text) {
                const head = view.state.selection.main.head;
                view.dispatch({
                    changes: {
                        from: head,
                        insert: text
                    },
                    selection: {
                        anchor: head + text.length
                    }
                });
            },
            placeCaretInViewport () {
                const vp = view.viewport;
                const seg = gapTable(view.state).segs.find((s)=>s.kind === "loaded" && s.from >= vp.from && s.to <= vp.to);
                if (seg) view.dispatch({
                    selection: {
                        anchor: seg.from
                    }
                });
            },
            undoDepth: ()=>(0,commands_dist/* undoDepth */.mk)(view.state),
            stats: ()=>({
                    loadedChars: loadedChars(view.state),
                    segs: gapTable(view.state).segs.length,
                    heap: performance.memory?.usedJSHeapSize ?? -1,
                    dirtySegs: gapTable(view.state).segs.filter((s)=>s.dirty).length
                }),
            composeProbe () {
                Object.defineProperty(view, "composing", {
                    get: ()=>true,
                    configurable: true
                });
                const before = view.state.doc.length;
                loadChunk(view, vdoc, 700000);
                const dispatched = view.state.doc.length !== before;
                delete view.composing;
                return {
                    dispatchedWhileComposing: dispatched
                };
            }
        };
        spikeApi(api);
        setReady(true);
        return ()=>{
            view.destroy();
            cancelAnimationFrame(raf);
        };
    }, [
        onStats
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        ref: host,
        "data-testid": "lazydoc-editor",
        "data-ready": ready,
        style: {
            height: "60vh",
            border: "1px solid #888"
        }
    });
}

;// ./src/app/spike-lazydoc/windowSwap.ts
// THROWAWAY (Task 0.1.4c, ADR-036 S-T3) prototype a1: window-swap model. One window of the file per
// EditorState; reaching a window edge swaps in a new state centred on the viewport (line-offset gutter).
// A window with unsaved edits is never swapped, because history cannot follow the document to a new state.




const WINDOW_LINES = 2000;
const EDGE_LINES = 200;
function windowExtensions(start) {
    return [
        (0,commands_dist/* history */.b6)(),
        (0,view_dist/* lineNumbers */.$K)({
            formatNumber: (n)=>String(start + n)
        }),
        // setState drops the initial state's theme, so the host-height rule is repeated here.
        view_dist/* EditorView */.Lz.theme({
            "&": {
                height: "100%"
            },
            ".cm-line": {
                height: `${LINE_HEIGHT}px`,
                lineHeight: `${LINE_HEIGHT}px`
            }
        })
    ];
}
function createWindowState(vdoc, start, size = WINDOW_LINES) {
    const lines = vdoc.fetch(start, size);
    return dist/* EditorState */.$t.create({
        doc: lines.join("\n"),
        extensions: windowExtensions(start)
    });
}
class WindowController {
    constructor(view, vdoc){
        this.view = view;
        this.vdoc = vdoc;
        this.start = 0;
        this.swaps = 0;
        this.dirty = false;
        this./** True from setState until the exact scroll offset is written; scroll ticks in between are transient. */ settling = false;
        view.setState(createWindowState(vdoc, 0));
    }
    /** 1-based file line at the top of the viewport. */ topAbs() {
        const block = this.view.lineBlockAtHeight(this.view.scrollDOM.getBoundingClientRect().top - this.view.documentTop);
        return this.start + this.view.state.doc.lineAt(block.from).number;
    }
    markDirtyFromHistory() {
        this.dirty = (0,commands_dist/* undoDepth */.mk)(this.view.state) > 0;
    }
    /** Swaps to a window containing file line `abs` (1-based) at the top of the viewport. */ swapTo(abs) {
        if (this.view.composing) return "refused-composing";
        this.markDirtyFromHistory();
        if (this.dirty) return "refused-dirty";
        const total = this.vdoc.lineCount;
        const start = Math.max(0, Math.min(total - WINDOW_LINES, abs - 1 - WINDOW_LINES / 2));
        this.start = start;
        this.view.setState(createWindowState(this.vdoc, start));
        // setState resets CodeMirror's scroll anchor to the document start on its next measure, so the exact
        // offset (fixed 20 px lines) is written from a measure callback, after that reset.
        const target = (abs - start - 1) * LINE_HEIGHT;
        this.view.requestMeasure({
            read: ()=>null,
            write: ()=>{
                const scroller = this.view.scrollDOM;
                scroller.scrollTop = target;
                // CodeMirror re-anchors once it has measured the new window's real line heights, so the offset is
                // re-asserted on the next two frames before edge detection is re-armed.
                requestAnimationFrame(()=>{
                    scroller.scrollTop = target;
                    requestAnimationFrame(()=>{
                        scroller.scrollTop = target;
                        this.settling = false;
                    });
                });
            }
        });
        this.swaps++;
        this.settling = true;
        return "swapped";
    }
    /** Edge-triggered: swap only when the viewport top is within EDGE_LINES of a window edge. */ maybeSwap() {
        if (this.settling) return "not-at-edge";
        const top = this.topAbs();
        const rel = top - this.start;
        const atTopEdge = rel < EDGE_LINES && this.start > 0;
        const atBottomEdge = rel > WINDOW_LINES - EDGE_LINES && this.start + WINDOW_LINES < this.vdoc.lineCount;
        return atTopEdge || atBottomEdge ? this.swapTo(top) : "not-at-edge";
    }
    /** Window position to 0-based file line and UTF-16 column (the oracle's coordinates). */ posToAbs(pos) {
        const dl = this.view.state.doc.lineAt(pos);
        return {
            line: this.start + dl.number - 1,
            col: pos - dl.from
        };
    }
}

;// ./src/app/spike-lazydoc/A1Host.tsx
/* __next_internal_client_entry_do_not_use__ A1Host auto */ 
// THROWAWAY (Task 0.1.4c): mounts prototype a1 (window-swap) over the same fake file.








function A1Host({ onStats }) {
    const host = (0,react.useRef)(null);
    const [ready, setReady] = (0,react.useState)(false);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!host.current) return;
        const vdoc = new VirtualDoc();
        const view = new view_dist/* EditorView */.Lz({
            state: dist/* EditorState */.$t.create({
                doc: "",
                extensions: [
                    view_dist/* EditorView */.Lz.theme({
                        "&": {
                            height: "100%"
                        }
                    })
                ]
            }),
            parent: host.current
        });
        const ctl = new WindowController(view, vdoc);
        let raf = 0;
        const tick = ()=>{
            raf = 0;
            const r = ctl.maybeSwap();
            onStats(`a1 top=${ctl.topAbs()} window=${ctl.start + 1}..${ctl.start + 2000} swaps=${ctl.swaps} last=${r} undo=${(0,commands_dist/* undoDepth */.mk)(view.state)}`);
        };
        const schedule = ()=>{
            if (!raf) raf = requestAnimationFrame(tick);
        };
        view.scrollDOM.addEventListener("scroll", schedule, {
            passive: true
        });
        const api = {
            lineHeight: ()=>LINE_HEIGHT,
            topAbs: ()=>ctl.topAbs(),
            topLineClientTop: ()=>domTopLineClientTop(view),
            scrollToAbs (n) {
                const rel = n - ctl.start;
                if (rel < 1 || rel > view.state.doc.lines) ctl.swapTo(n);
                else view.dispatch({
                    effects: view_dist/* EditorView */.Lz.scrollIntoView(view.state.doc.line(rel).from, {
                        y: "start"
                    })
                });
            },
            loadAboveViewport () {
                const before = {
                    top: ctl.topAbs(),
                    y: domTopLineClientTop(view)
                };
                return {
                    ok: ctl.swapTo(Math.max(1, before.top)) === "swapped",
                    before
                };
            },
            cycles (n) {
                const top = ctl.topAbs();
                for(let c = 0; c < n; c++)ctl.swapTo(top + (c % 2 ? 0 : 1500));
                ctl.swapTo(top);
            },
            type (text) {
                const head = view.state.selection.main.head;
                view.dispatch({
                    changes: {
                        from: head,
                        insert: text
                    },
                    selection: {
                        anchor: head + text.length
                    }
                });
            },
            placeCaretInViewport () {
                const b = view.lineBlockAtHeight(view.scrollDOM.getBoundingClientRect().top - view.documentTop + 200);
                view.dispatch({
                    selection: {
                        anchor: b.from
                    }
                });
            },
            undoDepth: ()=>(0,commands_dist/* undoDepth */.mk)(view.state),
            stats: ()=>({
                    loadedChars: view.state.doc.length,
                    segs: 1,
                    heap: performance.memory?.usedJSHeapSize ?? -1,
                    dirtySegs: (0,commands_dist/* undoDepth */.mk)(view.state) > 0 ? 1 : 0
                }),
            composeProbe () {
                Object.defineProperty(view, "composing", {
                    get: ()=>true,
                    configurable: true
                });
                const swaps = ctl.swaps;
                ctl.swapTo(700000);
                const dispatched = ctl.swaps !== swaps;
                delete view.composing;
                return {
                    dispatchedWhileComposing: dispatched
                };
            }
        };
        spikeApi(api);
        setReady(true);
        schedule();
        return ()=>{
            view.destroy();
            cancelAnimationFrame(raf);
        };
    }, [
        onStats
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        ref: host,
        "data-testid": "lazydoc-editor",
        "data-ready": ready,
        style: {
            height: "60vh",
            border: "1px solid #888"
        }
    });
}

// EXTERNAL MODULE: ./src/components/sessions/files/engine/CmEngine.ts + 21 modules
var CmEngine = __webpack_require__(40918);
// EXTERNAL MODULE: ./src/components/sessions/files/engine/cm6LazyDoc/gapTable.ts
var cm6LazyDoc_gapTable = __webpack_require__(95639);
;// ./src/app/spike-lazydoc/EngineHost.tsx
/* __next_internal_client_entry_do_not_use__ EngineHost auto */ 
// Regression host (Task 1.6.4c): the Task 0.1.4c spike surface driven through the production CmEngine
// (cm6LazyDoc) instead of the throwaway gapLine prototype. ?m=engine







/** Gutter number of the first line touching the scroller top; -1 while a placeholder covers it. */ function EngineHost_topAbs(root) {
    const scroller = root.querySelector(".cm-scroller");
    const top = scroller.getBoundingClientRect().top;
    const gap = Array.from(root.querySelectorAll(".cm-lazy-gap")).find((g)=>{
        const r = g.getBoundingClientRect();
        return r.top <= top && r.bottom > top;
    });
    if (gap) return -1;
    for (const el of Array.from(root.querySelectorAll(".cm-lineNumbers .cm-gutterElement"))){
        if (el.getBoundingClientRect().bottom > top && /^\d+$/.test(el.textContent ?? "")) return Number(el.textContent);
    }
    return -1;
}
function EngineHost({ onStats }) {
    const host = (0,react.useRef)(null);
    const [ready, setReady] = (0,react.useState)(false);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const root = host.current;
        if (!root) return;
        let handle = null;
        let cancelled = false;
        let tick;
        const vdoc = new VirtualDoc();
        const give = (r)=>handle?.provideRange(r, vdoc.fetch(r.startLine - 1, r.endLine - r.startLine + 1).join("\n"));
        const view = ()=>view_dist/* EditorView */.Lz.findFromDOM(root);
        (async ()=>{
            const engine = new CmEngine.CmEngine();
            const config = {
                mode: "Edit",
                ariaLabel: "File: spike"
            };
            const buffer = await engine.createBuffer({
                doc: "",
                lazy: {
                    lineCount: vdoc.lineCount
                },
                config
            });
            if (cancelled) return;
            handle = engine.mount(root, {
                buffer,
                ...config
            });
            handle.onNeedRange(give);
            const chunkAt = (abs)=>{
                const startLine = Math.floor(abs / cm6LazyDoc_gapTable/* CHUNK_LINES */.uR) * cm6LazyDoc_gapTable/* CHUNK_LINES */.uR + 1;
                return {
                    startLine,
                    endLine: startLine + cm6LazyDoc_gapTable/* CHUNK_LINES */.uR - 1
                };
            };
            const api = {
                lineHeight: ()=>cm6LazyDoc_gapTable/* LINE_HEIGHT */.$g,
                topAbs: ()=>EngineHost_topAbs(root),
                topLineClientTop: ()=>domTopLineClientTop({
                        scrollDOM: root.querySelector(".cm-scroller")
                    }),
                scrollToAbs: (n)=>handle?.reveal(n),
                loadAboveViewport (linesAbove) {
                    const before = {
                        top: EngineHost_topAbs(root),
                        y: domTopLineClientTop({
                            scrollDOM: root.querySelector(".cm-scroller")
                        })
                    };
                    const range = chunkAt(Math.max(0, before.top - linesAbove - 1));
                    const chars = (0,cm6LazyDoc_gapTable/* loadedChars */.z9)(view().state);
                    give(range);
                    return {
                        ok: (0,cm6LazyDoc_gapTable/* loadedChars */.z9)(view().state) !== chars,
                        before
                    };
                },
                cycles (n) {
                    const start = Math.max(EngineHost_topAbs(root), 1000);
                    for(let c = 0; c < n; c++){
                        const range = chunkAt(start + 20000 + c % 40 * 500);
                        give(range);
                        handle?.evictRange(range);
                    }
                },
                type: (text)=>void handle?.insertText(text),
                placeCaretInViewport () {
                    const line = root.querySelector(".cm-line:not(:has(.cm-lazy-gap))");
                    if (line) view().dispatch({
                        selection: {
                            anchor: view().posAtDOM(line)
                        }
                    });
                },
                undoDepth: ()=>(0,commands_dist/* undoDepth */.mk)(view().state),
                stats: ()=>({
                        loadedChars: (0,cm6LazyDoc_gapTable/* loadedChars */.z9)(view().state),
                        segs: (0,cm6LazyDoc_gapTable/* gapTableOf */.kl)(view().state).segs.length,
                        heap: performance.memory?.usedJSHeapSize ?? -1,
                        dirtySegs: (0,commands_dist/* undoDepth */.mk)(view().state) > 0 ? 1 : 0
                    }),
                composeProbe () {
                    const v = view();
                    Object.defineProperty(v, "compositionStarted", {
                        get: ()=>true,
                        configurable: true
                    });
                    const before = v.state.doc.length;
                    give({
                        startLine: 700000,
                        endLine: 700000 + cm6LazyDoc_gapTable/* CHUNK_LINES */.uR - 1
                    });
                    const dispatched = v.state.doc.length !== before;
                    delete v.compositionStarted;
                    return {
                        dispatchedWhileComposing: dispatched
                    };
                }
            };
            spikeApi(api);
            setReady(true);
            tick = setInterval(()=>onStats(`engine top=${EngineHost_topAbs(root)} loaded=${((0,cm6LazyDoc_gapTable/* loadedChars */.z9)(view().state) / 1048576).toFixed(2)}MiB`), 500);
        })();
        return ()=>{
            cancelled = true;
            clearInterval(tick);
            handle?.destroy();
        };
    }, [
        onStats
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        ref: host,
        "data-testid": "lazydoc-editor",
        "data-ready": ready,
        style: {
            height: "60vh",
            border: "1px solid #888"
        }
    });
}

;// ./src/app/spike-lazydoc/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 
// analytics-exempt
// THROWAWAY (Task 0.1.4c, ADR-036 S-T3): phone and Playwright page for the lazy-document models.
// ?m=a2 (gap-line prototype, default), ?m=a1 (window-swap prototype) or ?m=engine (production CmEngine, Task 1.6.4c).
// The prototypes are deleted after the owner sitting (Task 0.1.4d).




function SpikeLazyDocPage() {
    const [model, setModel] = (0,react.useState)(null);
    const [stats, setStats] = (0,react.useState)("");
    const [goto, setGoto] = (0,react.useState)("500000");
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        setModel(new URLSearchParams(window.location.search).get("m") ?? "a2");
    }, []);
    const onStats = (0,react.useCallback)((s)=>setStats(s), []);
    if (!model) return null;
    const jump = ()=>{
        const api = window.__spike?.api;
        api?.scrollToAbs(Number.parseInt(goto, 10));
    };
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        "data-testid": `lazydoc-model-${model}`,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("h2", {
                children: [
                    "Lazy document spike: ",
                    model === "a2" ? "a2 gap-line" : model === "engine" ? "production engine" : "a1 window-swap",
                    " (1,000,000 lines, about 50 MB virtual)"
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("a", {
                        href: "?m=a2",
                        children: "a2"
                    }),
                    " | ",
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("a", {
                        href: "?m=a1",
                        children: "a1"
                    }),
                    " | ",
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("a", {
                        href: "?m=engine",
                        children: "engine"
                    }),
                    ". Scroll fast, jump to a far line, then tap and type. Record PASS or FAIL for this model."
                ]
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
                style: {
                    minHeight: 44
                },
                onClick: jump,
                children: "Go"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                "data-testid": "lazydoc-stats",
                style: {
                    fontFamily: "monospace",
                    fontSize: 12
                },
                children: stats
            }),
            model === "a1" ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)(A1Host, {
                onStats: onStats
            }) : model === "engine" ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)(EngineHost, {
                onStats: onStats
            }) : /*#__PURE__*/ (0,react_jsx_runtime.jsx)(A2Host, {
                onStats: onStats
            })
        ]
    });
}


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

/***/ 26318:
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
const page4 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 66411));


// We inject the tree and pages here so that we can use them in the route
// module.
const tree = {
        children: [
        '',
        {
        children: [
        'spike-lazydoc',
        {
        children: ['__PAGE__', {}, {
          page: [page4, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/spike-lazydoc/page.tsx"],
          
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
const pages = ["/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/spike-lazydoc/page.tsx"];


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
        page: "/spike-lazydoc/page",
        pathname: "/spike-lazydoc",
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

/***/ 29294:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/work-async-storage.external.js");

/***/ }),

/***/ 32533:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 66411));


/***/ }),

/***/ 33873:
/***/ ((module) => {

"use strict";
module.exports = require("path");

/***/ }),

/***/ 46101:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 17439));


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

/***/ 66411:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": () => (__WEBPACK_DEFAULT_EXPORT__)
/* harmony export */ });
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(70679);
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__);

/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = ((0,react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__.registerClientReference)(
function() { throw new Error("Attempted to call the default export of \"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/spike-lazydoc/page.tsx\" from the server, but it's on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component."); },
"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/spike-lazydoc/page.tsx",
"default",
));


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

/***/ 76312:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Jq: () => (/* binding */ lineBackgroundModify),
/* harmony export */   Lt: () => (/* binding */ codeEditorRoot),
/* harmony export */   Lw: () => (/* binding */ gutterMarkerDelete),
/* harmony export */   NA: () => (/* binding */ degradeNotice),
/* harmony export */   VK: () => (/* binding */ codeEditor),
/* harmony export */   d9: () => (/* binding */ gutterMarkerAdd),
/* harmony export */   eD: () => (/* binding */ gutterMarkerModify),
/* harmony export */   kW: () => (/* binding */ lineBackgroundAdd),
/* harmony export */   ms: () => (/* binding */ lineBackgroundDelete)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(13507);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FCodeEditor_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xZW01bTRkMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGhlaWdodDogMTAwJTsKICBtaW4taGVpZ2h0OiAwOwp9Ci5fMWVtNW00ZDEgewogIGZsZXg6IDE7CiAgbWluLWhlaWdodDogMDsKICBoZWlnaHQ6IDEwMCU7Cn0KLl8xZW01bTRkMiB7CiAgZmxleC1zaHJpbms6IDA7CiAgcGFkZGluZzogNHB4IDEycHg7CiAgZm9udC1zaXplOiAxMnB4OwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6eSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6eik7CiAgYm9yZGVyLWJvdHRvbTogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6eCk7Cn0KLl8xZW01bTRkMSAuY20tZWRpdG9yIHsKICBoZWlnaHQ6IDEwMCU7Cn0KLl8xZW01bTRkMSAuY20tY2hhbmdlR3V0dGVyIHsKICB3aWR0aDogNHB4Owp9Ci5fMWVtNW00ZDMgewogIHdpZHRoOiA0cHg7CiAgaGVpZ2h0OiAxMDAlOwp9Ci5fMWVtNW00ZDQgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MXopOwp9Ci5fMWVtNW00ZDUgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MjApOwp9Ci5fMWVtNW00ZDYgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MXYpOwp9Ci5fMWVtNW00ZDcgewogIGJhY2tncm91bmQtaW1hZ2U6IGxpbmVhci1ncmFkaWVudCh2YXIoLS1fMTllejJyejF4KSwgdmFyKC0tXzE5ZXoycnoxeCkpOwp9Ci5fMWVtNW00ZDggewogIGJhY2tncm91bmQtaW1hZ2U6IGxpbmVhci1ncmFkaWVudCh2YXIoLS1fMTllejJyejF5KSwgdmFyKC0tXzE5ZXoycnoxeSkpOwp9Ci5fMWVtNW00ZDkgewogIGJhY2tncm91bmQtaW1hZ2U6IGxpbmVhci1ncmFkaWVudCh2YXIoLS1fMTllejJyejF3KSwgdmFyKC0tXzE5ZXoycnoxdykpOwp9_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(87892);


var codeEditor = '_1em5m4d1';
var codeEditorRoot = '_1em5m4d0';
var degradeNotice = '_1em5m4d2';
var gutterMarkerAdd = '_1em5m4d4 _1em5m4d3';
var gutterMarkerDelete = '_1em5m4d5 _1em5m4d3';
var gutterMarkerModify = '_1em5m4d6 _1em5m4d3';
var lineBackgroundAdd = '_1em5m4d7';
var lineBackgroundDelete = '_1em5m4d8';
var lineBackgroundModify = '_1em5m4d9';

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

/***/ 87892:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._1em5m4d0 {\n  display: flex;\n  flex-direction: column;\n  height: 100%;\n  min-height: 0;\n}\n._1em5m4d1 {\n  flex: 1 1;\n  min-height: 0;\n  height: 100%;\n}\n._1em5m4d2 {\n  flex-shrink: 0;\n  padding: 4px 12px;\n  font-size: 12px;\n  background: var(--_19ez2rzy);\n  color: var(--_19ez2rzz);\n  border-bottom: 1px solid var(--_19ez2rzx);\n}\n._1em5m4d1 .cm-editor {\n  height: 100%;\n}\n._1em5m4d1 .cm-changeGutter {\n  width: 4px;\n}\n._1em5m4d3 {\n  width: 4px;\n  height: 100%;\n}\n._1em5m4d4 {\n  background: var(--_19ez2rz1z);\n}\n._1em5m4d5 {\n  background: var(--_19ez2rz20);\n}\n._1em5m4d6 {\n  background: var(--_19ez2rz1v);\n}\n._1em5m4d7 {\n  background-image: linear-gradient(var(--_19ez2rz1x), var(--_19ez2rz1x));\n}\n._1em5m4d8 {\n  background-image: linear-gradient(var(--_19ez2rz1y), var(--_19ez2rz1y));\n}\n._1em5m4d9 {\n  background-image: linear-gradient(var(--_19ez2rz1w), var(--_19ez2rz1w));\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ })

};
;

// load runtime
var __webpack_require__ = require("../../webpack-runtime.js");
__webpack_require__.C(exports);
var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
var __webpack_exports__ = __webpack_require__.X(0, [4655,9154,8513,4164,918], () => (__webpack_exec__(26318)));
module.exports = __webpack_exports__;

})();