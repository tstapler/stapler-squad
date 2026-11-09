"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[2086],{

/***/ 50091:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $g: () => (/* binding */ LINE_HEIGHT),
/* harmony export */   AP: () => (/* binding */ isLazyState),
/* harmony export */   Ft: () => (/* binding */ gapTableField),
/* harmony export */   I0: () => (/* binding */ MAX_GAP_LINES),
/* harmony export */   I2: () => (/* binding */ initialDocFor),
/* harmony export */   Kx: () => (/* binding */ PLACEHOLDER),
/* harmony export */   b1: () => (/* binding */ splitGap),
/* harmony export */   fm: () => (/* binding */ segAtFileLine),
/* harmony export */   jc: () => (/* binding */ lazyLoad),
/* harmony export */   kl: () => (/* binding */ gapTableOf),
/* harmony export */   l7: () => (/* binding */ segIndexAtPos),
/* harmony export */   pM: () => (/* binding */ initialSegs),
/* harmony export */   tq: () => (/* binding */ setSegs),
/* harmony export */   uR: () => (/* binding */ CHUNK_LINES),
/* harmony export */   xf: () => (/* binding */ isLazyLoad),
/* harmony export */   yi: () => (/* binding */ absLineOfDocLine),
/* harmony export */   z9: () => (/* binding */ loadedChars)
/* harmony export */ });
/* unused harmony export withPrefix */
/* harmony import */ var _codemirror_state__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(24183);
// Gap table of the cm6LazyDoc gap-line model (ADR-036 decision 19): the document holds loaded lines plus one
// placeholder line per unloaded gap; this StateField maps document positions to file lines.

const LINE_HEIGHT = 20;
const CHUNK_LINES = 500;
/** One placeholder never spans more than this many lines: a 250k-line (5M px) block defeated CM6's height scaler. */ const MAX_GAP_LINES = 5000;
/** Not a "special char": basicSetup's highlightSpecialChars would otherwise wrap it in a plugin decoration that clashes with the block widget. */ const PLACEHOLDER = " ";
const setSegs = _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .StateEffect */ .Pe.define();
/** Marks a load/evict transaction: guards let it through and the buffer treats it as non-user. */ const lazyLoad = _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .StateEffect */ .Pe.define();
const isLazyLoad = (tr)=>tr.effects.some((e)=>e.is(lazyLoad));
/** Segments the field starts from; a buffer supplies it once, when its state is created. */ const initialSegs = _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .Facet */ .sj.define({
    combine: (v)=>{
        var _v_;
        return (_v_ = v[0]) !== null && _v_ !== void 0 ? _v_ : [];
    }
});
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
    return withPrefix(table.segs.map((s)=>{
        const from = tr.changes.mapPos(s.from, -1);
        const to = tr.changes.mapPos(s.to, 1);
        const lines = s.kind === "gap" ? s.lines : doc.lineAt(to).number - doc.lineAt(from).number + 1;
        return {
            ...s,
            from,
            to,
            lines
        };
    }));
}
const gapTableField = _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .StateField */ .sU.define({
    create: (state)=>withPrefix(state.facet(initialSegs)),
    update (table, tr) {
        for (const e of tr.effects)if (e.is(setSegs)) return withPrefix(e.value);
        return tr.docChanged ? remap(table, tr) : table;
    }
});
const gapTableOf = (state)=>state.field(gapTableField);
const isLazyState = (state)=>state.field(gapTableField, false) !== undefined;
/** Initial gap pieces covering `total` lines, each at most MAX_GAP_LINES, one placeholder line apiece. */ function splitGap(total) {
    const segs = [];
    let pos = 0;
    for(let done = 0; done < total; done += MAX_GAP_LINES){
        segs.push({
            kind: "gap",
            from: pos,
            to: pos + 1,
            lines: Math.min(MAX_GAP_LINES, total - done)
        });
        pos += 2;
    }
    return segs;
}
const initialDocFor = (segs)=>segs.map(()=>PLACEHOLDER).join("\n");
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
/** 1-based file line shown in the gutter for a 1-based document line. */ function absLineOfDocLine(state, docLine) {
    // The gutter may format a line number from a document one step behind the state it is handed.
    if (docLine > state.doc.lines) return docLine;
    const table = gapTableOf(state);
    const i = segIndexAtPos(table, state.doc.line(docLine).from);
    const seg = table.segs[i];
    if (seg.kind === "gap") return table.prefix[i] + 1;
    return table.prefix[i] + (docLine - state.doc.lineAt(seg.from).number) + 1;
}
function loadedChars(state) {
    return gapTableOf(state).segs.reduce((n, s)=>s.kind === "loaded" ? n + (s.to - s.from) : n, 0);
}
/** Index of the segment holding 0-based current-file line `abs`, and the line's offset inside it. */ function segAtFileLine(table, abs) {
    for(let i = 0; i < table.segs.length; i++){
        const start = table.prefix[i];
        if (abs >= start && abs < start + table.segs[i].lines) return {
            index: i,
            offset: abs - start
        };
    }
    return null;
}


/***/ }),

/***/ 92086:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  CmEngine: () => (/* binding */ CmEngine)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+view@6.41.0/node_modules/@codemirror/view/dist/index.js
var dist = __webpack_require__(19421);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+state@6.6.0/node_modules/@codemirror/state/dist/index.js + 1 modules
var state_dist = __webpack_require__(24183);
// EXTERNAL MODULE: ./src/components/sessions/CodeEditor.css.ts
var CodeEditor_css = __webpack_require__(57764);
;// ./src/components/sessions/files/extensions/readOnlyFacet.ts

// Story 2.2.1 grows this into the read-only facet (reason, banner state); for now it is the plain switch.
// editable stays true so the browser still allows text selection and copy.
function readOnlyExtension(readOnly) {
    return state_dist/* EditorState */.$t.readOnly.of(readOnly);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/codemirror@6.0.2/node_modules/codemirror/dist/index.js + 1 modules
var codemirror_dist = __webpack_require__(90824);
;// ./src/components/sessions/files/extensions/degrade.ts


/** basicSetup minus what costs on a phone: fold gutter, bracket matching, active line, autocompletion, search keymap. */ const lightSetup = ()=>[
        (0,dist/* lineNumbers */.$K)(),
        codemirror_dist/* minimalSetup */.V0
    ];
/** The editor's base feature set for this buffer; a degraded buffer also skips its language parser (see CmEngine). */ function setupFor(profile) {
    return (profile === null || profile === void 0 ? void 0 : profile.degraded) ? lightSetup() : codemirror_dist/* basicSetup */.oQ;
}
/** Marks a degraded buffer so tests and styles can tell. */ function degradeExtension(profile) {
    return (profile === null || profile === void 0 ? void 0 : profile.degraded) ? dist/* EditorView */.Lz.contentAttributes.of({
        "data-degraded": "true"
    }) : [];
}

;// ./src/components/sessions/files/extensions/searchHighlight.ts


/** Marks (or, with null, clears) the matched text of the hit that was just opened. */ const setSearchHit = state_dist/* StateEffect */.Pe.define();
const mark = dist/* Decoration */.NZ.mark({
    class: "cm-search-hit"
});
const field = state_dist/* StateField */.sU.define({
    create: ()=>dist/* Decoration */.NZ.none,
    update (deco, tr) {
        for (const e of tr.effects){
            if (!e.is(setSearchHit)) continue;
            if (!e.value) return dist/* Decoration */.NZ.none;
            const line = tr.state.doc.line(Math.min(Math.max(1, e.value.line), tr.state.doc.lines));
            const from = Math.min(line.from + Math.max(0, e.value.from), line.to);
            const to = Math.min(line.from + Math.max(0, e.value.to), line.to);
            return from < to ? dist/* Decoration */.NZ.set([
                mark.range(from, to)
            ]) : dist/* Decoration */.NZ.none;
        }
        if (tr.docChanged) return deco.map(tr.changes);
        // Transient: the user moving the cursor or selecting ends the highlight.
        if (tr.selection && tr.isUserEvent("select")) return dist/* Decoration */.NZ.none;
        return deco;
    },
    provide: (f)=>dist/* EditorView */.Lz.decorations.from(f)
});
const theme = dist/* EditorView */.Lz.baseTheme({
    ".cm-search-hit": {
        backgroundColor: "rgba(245, 158, 11, 0.45)",
        outline: "1px solid rgba(245, 158, 11, 0.9)",
        borderRadius: "2px"
    }
});
function searchHighlightExtension() {
    return [
        field,
        theme
    ];
}

;// ./src/components/sessions/files/extensions/vim.ts
// Story 2.4.1 fills in @replit/codemirror-vim and removes the legacy keymap.
function vimExtension() {
    return [];
}

;// ./src/components/sessions/files/extensions/merge.ts
// Story 2.3.2 fills in the 3-way merge surface.
function mergeExtension() {
    return [];
}

;// ./src/components/sessions/files/engine/languages.ts
async function loadCodemirrorLang(lang) {
    switch(lang){
        case "javascript":
        case "jsx":
        case "typescript":
        case "tsx":
            {
                const { javascript } = await Promise.all(/* import() */[__webpack_require__.e(9163), __webpack_require__.e(7680)]).then(__webpack_require__.bind(__webpack_require__, 67680));
                const isTs = lang === "typescript" || lang === "tsx";
                const isJsx = lang === "jsx" || lang === "tsx";
                return javascript({
                    typescript: isTs,
                    jsx: isJsx
                });
            }
        case "python":
            {
                const { python } = await Promise.all(/* import() */[__webpack_require__.e(9163), __webpack_require__.e(9756)]).then(__webpack_require__.bind(__webpack_require__, 29756));
                return python();
            }
        case "go":
            {
                const { go } = await Promise.all(/* import() */[__webpack_require__.e(9163), __webpack_require__.e(8296)]).then(__webpack_require__.bind(__webpack_require__, 15915));
                return go();
            }
        case "markdown":
            {
                const { markdown } = await Promise.all(/* import() */[__webpack_require__.e(9163), __webpack_require__.e(7680), __webpack_require__.e(4945), __webpack_require__.e(7041), __webpack_require__.e(2922)]).then(__webpack_require__.bind(__webpack_require__, 32922));
                return markdown();
            }
        case "json":
            {
                const { json } = await Promise.all(/* import() */[__webpack_require__.e(9163), __webpack_require__.e(3916)]).then(__webpack_require__.bind(__webpack_require__, 93916));
                return json();
            }
        case "html":
            {
                const { html } = await Promise.all(/* import() */[__webpack_require__.e(9163), __webpack_require__.e(7680), __webpack_require__.e(4945), __webpack_require__.e(7041)]).then(__webpack_require__.bind(__webpack_require__, 17041));
                return html();
            }
        case "css":
        case "scss":
            {
                const { css } = await Promise.all(/* import() */[__webpack_require__.e(9163), __webpack_require__.e(4945)]).then(__webpack_require__.bind(__webpack_require__, 84945));
                return css();
            }
        case "rust":
            {
                const { rust } = await Promise.all(/* import() */[__webpack_require__.e(9163), __webpack_require__.e(4888)]).then(__webpack_require__.bind(__webpack_require__, 64888));
                return rust();
            }
        case "java":
            {
                const { java } = await Promise.all(/* import() */[__webpack_require__.e(9163), __webpack_require__.e(1224)]).then(__webpack_require__.bind(__webpack_require__, 71224));
                return java();
            }
        default:
            return null;
    }
}

// EXTERNAL MODULE: ./src/components/sessions/files/engine/cm6LazyDoc/gapTable.ts
var gapTable = __webpack_require__(50091);
;// ./src/components/sessions/files/engine/diffHighlight.ts



/**
 * The diff mark for a document line. A gap-line lazy document holds one placeholder per unloaded gap, so its
 * document line numbers are not file lines: map through the gap table and never mark a placeholder.
 * (Window-swap mode has no gap table and is unsupported; it is the unselected fallback.)
 */ function markAtDocLine(state, marks, docLine) {
    if (!(0,gapTable/* isLazyState */.AP)(state)) return marks.get(docLine);
    const table = (0,gapTable/* gapTableOf */.kl)(state);
    if (table.segs[(0,gapTable/* segIndexAtPos */.l7)(table, state.doc.line(docLine).from)].kind === "gap") return undefined;
    return marks.get((0,gapTable/* absLineOfDocLine */.yi)(state, docLine));
}
/** Tints changed lines to match their gutter bar. */ function diffHighlightExtension(marks) {
    if (!marks || marks.size === 0) return [];
    const deco = {
        add: dist/* Decoration */.NZ.line({
            class: CodeEditor_css/* lineBackgroundAdd */.kW
        }),
        delete: dist/* Decoration */.NZ.line({
            class: CodeEditor_css/* lineBackgroundDelete */.ms
        }),
        modify: dist/* Decoration */.NZ.line({
            class: CodeEditor_css/* lineBackgroundModify */.Jq
        })
    };
    const build = (view)=>{
        const out = [];
        let last = -1;
        for (const { from, to } of view.visibleRanges){
            let pos = Math.max(from, last + 1);
            while(pos <= to){
                const line = view.state.doc.lineAt(pos);
                const type = markAtDocLine(view.state, marks, line.number);
                if (type) out.push(deco[type].range(line.from));
                last = line.to;
                pos = line.to + 1;
            }
        }
        return dist/* Decoration */.NZ.set(out);
    };
    return dist/* ViewPlugin */.Z9.fromClass(class {
        update(u) {
            if (u.docChanged || u.viewportChanged) this.decorations = build(u.view);
        }
        constructor(view){
            this.decorations = build(view);
        }
    }, {
        decorations: (v)=>v.decorations
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+commands@6.10.3/node_modules/@codemirror/commands/dist/index.js
var commands_dist = __webpack_require__(52198);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+search@6.6.0/node_modules/@codemirror/search/dist/index.js
var search_dist = __webpack_require__(65678);
;// ./src/components/sessions/files/engine/doubleG.ts
// Vim-style "gg" needs two "g" presses in a short window; a lone "g" is swallowed.
// Kept CodeMirror-free so FileContentViewer can re-export it without loading the editor chunk.
const DOUBLE_G_WINDOW_MS = 400;
function isDoubleGPress(lastGPressAt, now) {
    let windowMs = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : DOUBLE_G_WINDOW_MS;
    return now - lastGPressAt < windowMs;
}

;// ./src/components/sessions/files/engine/legacyKeymap.ts




// Temporary hand-rolled hjkl keymap; Story 2.4.1 replaces it with @replit/codemirror-vim.
function legacyVimKeymap() {
    let lastGPress = 0;
    return dist/* keymap */.w4.of([
        {
            key: "h",
            run: commands_dist/* cursorCharLeft */.Cv
        },
        {
            key: "l",
            run: commands_dist/* cursorCharRight */.xt
        },
        {
            key: "j",
            run: commands_dist/* cursorLineDown */.HH
        },
        {
            key: "k",
            run: commands_dist/* cursorLineUp */.eg
        },
        {
            key: "0",
            run: commands_dist/* cursorLineBoundaryBackward */.My
        },
        {
            key: "$",
            run: commands_dist/* cursorLineBoundaryForward */.oj
        },
        {
            key: "g",
            run: (v)=>{
                const now = Date.now();
                const isDoubleG = isDoubleGPress(lastGPress, now);
                lastGPress = isDoubleG ? 0 : now;
                return isDoubleG ? (0,commands_dist/* cursorDocStart */.p6)(v) : true;
            }
        },
        {
            key: "G",
            run: commands_dist/* cursorDocEnd */.aS
        },
        {
            key: "Ctrl-d",
            run: commands_dist/* cursorPageDown */.sT
        },
        {
            key: "Ctrl-u",
            run: commands_dist/* cursorPageUp */.VA
        },
        {
            key: "Shift-h",
            run: commands_dist/* selectCharLeft */.QP
        },
        {
            key: "Shift-l",
            run: commands_dist/* selectCharRight */.Dt
        },
        {
            key: "Shift-j",
            run: commands_dist/* selectLineDown */.jK
        },
        {
            key: "Shift-k",
            run: commands_dist/* selectLineUp */.Mm
        },
        {
            key: "Shift-0",
            run: commands_dist/* selectLineBoundaryBackward */.aG
        },
        {
            key: "Shift-$",
            run: commands_dist/* selectLineBoundaryForward */.Q4
        },
        {
            key: "Shift-Ctrl-d",
            run: commands_dist/* selectPageDown */.wI
        },
        {
            key: "Shift-Ctrl-u",
            run: commands_dist/* selectPageUp */.Lo
        },
        {
            key: "y",
            run: (v)=>{
                const { from, to } = v.state.selection.main;
                const selected = v.state.sliceDoc(from, to);
                if (selected) void navigator.clipboard.writeText(selected);
                return true;
            }
        },
        {
            key: "/",
            run: search_dist/* openSearchPanel */.pi
        },
        {
            key: "n",
            run: search_dist/* findNext */.Hg
        },
        {
            key: "Shift-n",
            run: search_dist/* findPrevious */.ve
        }
    ]);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+language@6.12.3/node_modules/@codemirror/language/dist/index.js
var language_dist = __webpack_require__(3772);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@lezer+highlight@1.2.3/node_modules/@lezer/highlight/dist/index.js
var highlight_dist = __webpack_require__(60186);
// EXTERNAL MODULE: ./src/styles/theme.css.ts
var theme_css = __webpack_require__(90377);
;// ./src/components/sessions/files/engine/cmTheme.ts




// Editor chrome sourced from the theme-invariant terminal token family, matching FilesTab.css.ts's container.
const chrome = dist/* EditorView */.Lz.theme({
    "&": {
        color: theme_css.vars.color.terminalForeground,
        backgroundColor: theme_css.vars.color.terminalBackground,
        height: "100%"
    },
    ".cm-content": {
        caretColor: theme_css.vars.color.terminalCursor
    },
    ".cm-cursor, .cm-dropCursor": {
        borderLeftColor: theme_css.vars.color.terminalCursor
    },
    "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection": {
        backgroundColor: "".concat(theme_css.vars.color.terminalHoverBg, " !important")
    },
    ".cm-activeLine": {
        backgroundColor: theme_css.vars.color.terminalHoverBg
    },
    ".cm-activeLineGutter": {
        backgroundColor: theme_css.vars.color.terminalHoverBg,
        color: theme_css.vars.color.terminalForeground
    },
    ".cm-gutters": {
        backgroundColor: theme_css.vars.color.terminalBackground,
        color: theme_css.vars.color.terminalTextMuted,
        borderRight: "1px solid ".concat(theme_css.vars.color.terminalBorder)
    },
    ".cm-lineNumbers .cm-gutterElement": {
        color: theme_css.vars.color.terminalTextMuted
    }
}, {
    dark: true
});
// Syntax-token colors reuse the app's semantic tokens so they track the accent/status palette.
const tokens = language_dist/* HighlightStyle */.cr.define([
    {
        tag: [
            highlight_dist/* tags */._A.keyword,
            highlight_dist/* tags */._A.controlKeyword,
            highlight_dist/* tags */._A.operatorKeyword,
            highlight_dist/* tags */._A.modifier
        ],
        color: theme_css.vars.color.primary
    },
    {
        tag: [
            highlight_dist/* tags */._A.string,
            highlight_dist/* tags */._A.special(highlight_dist/* tags */._A.string)
        ],
        color: theme_css.vars.color.success
    },
    {
        tag: [
            highlight_dist/* tags */._A.comment,
            highlight_dist/* tags */._A.lineComment,
            highlight_dist/* tags */._A.blockComment
        ],
        color: theme_css.vars.color.terminalTextMuted,
        fontStyle: "italic"
    },
    {
        tag: [
            highlight_dist/* tags */._A.number,
            highlight_dist/* tags */._A.bool,
            highlight_dist/* tags */._A.null
        ],
        color: theme_css.vars.color.warning
    },
    {
        tag: [
            highlight_dist/* tags */._A.function(highlight_dist/* tags */._A.variableName),
            highlight_dist/* tags */._A.function(highlight_dist/* tags */._A.propertyName)
        ],
        color: theme_css.vars.color.accentText
    },
    {
        tag: [
            highlight_dist/* tags */._A.definition(highlight_dist/* tags */._A.variableName),
            highlight_dist/* tags */._A.definition(highlight_dist/* tags */._A.propertyName)
        ],
        color: theme_css.vars.color.terminalForeground
    },
    {
        tag: [
            highlight_dist/* tags */._A.className,
            highlight_dist/* tags */._A.typeName
        ],
        color: theme_css.vars.color.primary
    },
    {
        tag: [
            highlight_dist/* tags */._A.tagName,
            highlight_dist/* tags */._A.attributeName
        ],
        color: theme_css.vars.color.gitModified
    },
    {
        tag: highlight_dist/* tags */._A.invalid,
        color: theme_css.vars.color.error
    }
]);
const cmTheme = [
    chrome,
    (0,language_dist/* syntaxHighlighting */.y9)(tokens)
];

;// ./src/components/sessions/files/engine/pendingEdits.ts
function posAt(base, offset) {
    const line = base.lineAt(offset);
    return {
        line: line.number - 1,
        colUtf16: offset - line.from
    };
}
/** `changes` addresses `base`; the result is in `base` coordinates, ordered and non-overlapping. */ function changesToEdits(changes, base) {
    const edits = [];
    changes.iterChanges((fromA, toA, _fromB, _toB, inserted)=>{
        edits.push({
            start: posAt(base, fromA),
            end: posAt(base, toA),
            insert: inserted.sliceString(0, undefined, "\n")
        });
    });
    return edits;
}

;// ./src/components/sessions/files/engine/cm6LazyDoc/loadEvict.ts
// Load and evict as non-history, non-user transactions: undo depth and dirty state never see them.


const shift = (segs, delta)=>segs.map((s)=>({
            ...s,
            from: s.from + delta,
            to: s.to + delta
        }));
function dispatchLoad(view, change, segs) {
    view.dispatch({
        changes: change,
        effects: [
            gapTable/* setSegs */.tq.of(segs),
            gapTable/* lazyLoad */.jc.of(null)
        ],
        annotations: state_dist/* Transaction */.ZX.addToHistory.of(false)
    });
}
/** Replaces `count` lines of gap `index`, starting `offset` lines into it, with `text` (exactly `count` lines). */ function loadIntoGap(view, index, offset, text, count) {
    if (view.composing) return false;
    const table = (0,gapTable/* gapTableOf */.kl)(view.state);
    const gap = table.segs[index];
    if ((gap === null || gap === void 0 ? void 0 : gap.kind) !== "gap" || count < 1 || offset + count > gap.lines) return false;
    const pieces = [];
    if (offset > 0) pieces.push({
        text: gapTable/* PLACEHOLDER */.Kx,
        kind: "gap",
        lines: offset
    });
    pieces.push({
        text,
        kind: "loaded",
        lines: count
    });
    const rest = gap.lines - offset - count;
    if (rest > 0) pieces.push({
        text: gapTable/* PLACEHOLDER */.Kx,
        kind: "gap",
        lines: rest
    });
    let pos = gap.from;
    const fresh = pieces.map((p)=>{
        const seg = {
            kind: p.kind,
            from: pos,
            to: pos + p.text.length,
            lines: p.lines
        };
        pos += p.text.length + 1;
        return seg;
    });
    const insert = pieces.map((p)=>p.text).join("\n");
    const delta = insert.length - (gap.to - gap.from);
    dispatchLoad(view, {
        from: gap.from,
        to: gap.to,
        insert
    }, [
        ...table.segs.slice(0, index),
        ...fresh,
        ...shift(table.segs.slice(index + 1), delta)
    ]);
    return true;
}
/**
 * Turns `count` lines of loaded segment `index`, from `offset`, back into a placeholder, merging with a
 * neighboring gap while it stays under the cap. The caller has already checked the lines are clean.
 */ function evictFromLoaded(view, index, offset, count) {
    if (view.composing) return false;
    const { segs } = (0,gapTable/* gapTableOf */.kl)(view.state);
    const seg = segs[index];
    if ((seg === null || seg === void 0 ? void 0 : seg.kind) !== "loaded" || count < 1 || offset + count > seg.lines) return false;
    const doc = view.state.doc;
    const first = doc.lineAt(seg.from).number;
    const start = doc.line(first + offset).from;
    const end = doc.line(first + offset + count - 1).to;
    const keepLeft = offset;
    const keepRight = seg.lines - offset - count;
    let lines = count;
    let lo = index;
    let hi = index;
    const prev = segs[index - 1];
    if (keepLeft === 0 && (prev === null || prev === void 0 ? void 0 : prev.kind) === "gap" && lines + prev.lines <= gapTable/* MAX_GAP_LINES */.I0) {
        lo = index - 1;
        lines += prev.lines;
    }
    const next = segs[index + 1];
    if (keepRight === 0 && (next === null || next === void 0 ? void 0 : next.kind) === "gap" && lines + next.lines <= gapTable/* MAX_GAP_LINES */.I0) {
        hi = index + 1;
        lines += next.lines;
    }
    const changeFrom = lo < index ? segs[lo].from : start;
    const changeTo = hi > index ? segs[hi].to : end;
    const delta = gapTable/* PLACEHOLDER */.Kx.length - (changeTo - changeFrom);
    const fresh = [];
    if (keepLeft > 0) fresh.push({
        kind: "loaded",
        from: seg.from,
        to: start - 1,
        lines: keepLeft
    });
    fresh.push({
        kind: "gap",
        from: changeFrom,
        to: changeFrom + gapTable/* PLACEHOLDER */.Kx.length,
        lines
    });
    if (keepRight > 0) fresh.push({
        kind: "loaded",
        from: end + 1 + delta,
        to: seg.to + delta,
        lines: keepRight
    });
    dispatchLoad(view, {
        from: changeFrom,
        to: changeTo,
        insert: gapTable/* PLACEHOLDER */.Kx
    }, [
        ...segs.slice(0, lo),
        ...fresh,
        ...shift(segs.slice(hi + 1), delta)
    ]);
    return true;
}

;// ./src/components/sessions/files/engine/cm6LazyDoc/lazyPending.ts
// Edit bookkeeping for a lazy buffer. The pending/inflight change sets address `base`, the document at the
// last ack. Loads and evictions rewrite placeholders in the live document, so they are mapped into `base`
// and over both change sets; edits then leave in absolute file coordinates by adding the hidden gap lines.

function unacked(t) {
    var _t_inflight;
    return t.inflight && t.pending ? t.inflight.compose(t.pending) : (_t_inflight = t.inflight) !== null && _t_inflight !== void 0 ? _t_inflight : t.pending;
}
/** `load` was applied to the live document (base plus all unacked edits); carry it into base and the edit sets. */ function rebaseOverLoad(t, load) {
    const mid = t.inflight ? t.inflight.apply(t.base) : t.base;
    const loadMid = t.pending ? load.map(t.pending.invert(mid)) : load;
    const loadBase = t.inflight ? loadMid.map(t.inflight.invert(t.base)) : loadMid;
    const { inflight, pending } = t;
    t.base = loadBase.apply(t.base);
    t.inflight = inflight ? inflight.map(loadBase) : null;
    t.pending = pending ? pending.map(loadMid) : null;
}
/** Lines hidden behind placeholders that sit before live-document offset `pos`, minus the placeholder line itself. */ function hiddenBefore(segs, pos) {
    let hidden = 0;
    for (const s of segs)if (s.kind === "gap" && s.from < pos) hidden += s.lines - 1;
    return hidden;
}
/** Unacked edits as absolute (whole-file) line and UTF-16 column in the coordinates of the last-acked file. */ function absoluteEdits(t, state) {
    const changes = unacked(t);
    if (!changes) return [];
    const { segs } = (0,gapTable/* gapTableOf */.kl)(state);
    const posAt = (offset)=>{
        const line = t.base.lineAt(offset);
        const live = changes.mapPos(offset, -1);
        return {
            line: line.number - 1 + hiddenBefore(segs, live),
            colUtf16: offset - line.from
        };
    };
    const edits = [];
    changes.iterChanges((fromA, toA, _fromB, _toB, inserted)=>{
        edits.push({
            start: posAt(fromA),
            end: posAt(toA),
            insert: inserted.sliceString(0, undefined, "\n")
        });
    });
    return edits;
}
/** Whether any unacked edit touches the live-document range [from, to]; such a range must not be evicted. */ function isDirtyRange(t, from, to) {
    const changes = unacked(t);
    let dirty = false;
    changes === null || changes === void 0 ? void 0 : changes.iterChangedRanges((_a, _b, fromB, toB)=>{
        if (fromB <= to && toB >= from) dirty = true;
    });
    return dirty;
}
/** Start line (0-based) and line count of every segment in the last-acked file, derived from `base`. */ function baseLayout(t, state) {
    var _unacked;
    const inv = (_unacked = unacked(t)) === null || _unacked === void 0 ? void 0 : _unacked.invert(t.base);
    let start = 0;
    return (0,gapTable/* gapTableOf */.kl)(state).segs.map((s)=>{
        let count = s.lines;
        if (s.kind === "loaded") {
            const from = inv ? inv.mapPos(s.from, -1) : s.from;
            const to = inv ? inv.mapPos(s.to, 1) : s.to;
            count = t.base.lineAt(to).number - t.base.lineAt(from).number + 1;
        }
        const entry = {
            start,
            count
        };
        start += count;
        return entry;
    });
}

;// ./src/components/sessions/files/engine/cm6LazyDoc/gapLineController.ts




/** Resident chunk text above this many UTF-16 units is evicted back to placeholders (the phone budget). */ const RESIDENT_BUDGET_CHARS = 8 * 1024 * 1024;
const RETRY_AFTER_MS = 1000;
const keyOf = (r)=>"".concat(r.startLine, "-").concat(r.endLine);
/** Gap-line model (ADR-036 a2): host-facing ranges are 1-based inclusive lines of the last-acked file. */ class GapLineController {
    attach(track, info) {
        if (this.sink) this.sink.target = null;
        var _info_sink;
        const sink = (_info_sink = info === null || info === void 0 ? void 0 : info.sink) !== null && _info_sink !== void 0 ? _info_sink : null;
        this.track = track;
        this.sink = sink;
        this.asked.clear();
        this.revealTo = null;
        if (sink) sink.target = (demand)=>this.onDemand(demand);
    }
    onNeedRange(cb) {
        this.needCb = cb;
    }
    setFetcher(fetcher) {
        this.fetcher = fetcher;
    }
    absorb(tr) {
        if (!this.track || !(0,gapTable/* isLazyLoad */.xf)(tr)) return false;
        rebaseOverLoad(this.track, tr.changes);
        return true;
    }
    edits() {
        return this.track ? absoluteEdits(this.track, this.view.state) : [];
    }
    request(range) {
        var _this_fetcher, _this;
        const key = keyOf(range);
        const now = Date.now();
        var _this_asked_get;
        if (now - ((_this_asked_get = this.asked.get(key)) !== null && _this_asked_get !== void 0 ? _this_asked_get : -Infinity) < RETRY_AFTER_MS) return;
        this.asked.set(key, now);
        if (this.needCb) this.needCb(range);
        else void ((_this_fetcher = (_this = this).fetcher) === null || _this_fetcher === void 0 ? void 0 : _this_fetcher.call(_this, range).then((text)=>this.provideRange(range, text), ()=>this.asked.delete(key)));
    }
    onDemand(demand) {
        if (!this.track) return;
        const layout = baseLayout(this.track, this.view.state);
        for (const { index, offset } of demand){
            const { start, count } = layout[index];
            const first = Math.max(start, Math.floor((start + offset) / gapTable/* CHUNK_LINES */.uR) * gapTable/* CHUNK_LINES */.uR);
            this.request({
                startLine: first + 1,
                endLine: Math.min(start + count, first + gapTable/* CHUNK_LINES */.uR)
            });
        }
    }
    provideRange(range, text) {
        const track = this.track;
        if (!track) throw new Error("CmEngine: provideRange needs a lazy source");
        this.asked.delete(keyOf(range));
        const lines = text.split("\n");
        const wanted = range.endLine - range.startLine + 1;
        if (lines.length === wanted + 1 && lines[wanted] === "") lines.pop();
        const from = range.startLine - 1;
        const to = Math.min(from + lines.length, range.endLine);
        const layout = baseLayout(track, this.view.state);
        const { segs } = (0,gapTable/* gapTableOf */.kl)(this.view.state);
        // Back to front, so loading one gap never shifts the index of a gap still to be loaded.
        for(let i = segs.length - 1; i >= 0; i--){
            const { start, count } = layout[i];
            if (segs[i].kind !== "gap" || start >= to || start + count <= from) continue;
            const lo = Math.max(start, from);
            const hi = Math.min(start + count, to);
            loadIntoGap(this.view, i, lo - start, lines.slice(lo - from, hi - from).join("\n"), hi - lo);
        }
        this.enforceBudget();
        this.settleReveal();
    }
    evictRange(range) {
        const track = this.track;
        if (!track) throw new Error("CmEngine: evictRange needs a lazy source");
        const layout = baseLayout(track, this.view.state);
        const { segs } = (0,gapTable/* gapTableOf */.kl)(this.view.state);
        const from = range.startLine - 1;
        for(let i = segs.length - 1; i >= 0; i--){
            const { start, count } = layout[i];
            const s = segs[i];
            if (s.kind !== "loaded" || start >= range.endLine || start + count <= from) continue;
            if (isDirtyRange(track, s.from, s.to)) continue;
            const lo = Math.max(start, from);
            evictFromLoaded(this.view, i, lo - start, Math.min(start + count, range.endLine) - lo);
        }
    }
    /** Drops the clean loaded segment farthest from the viewport until resident text fits the budget. */ enforceBudget() {
        const track = this.track;
        while(track && (0,gapTable/* loadedChars */.z9)(this.view.state) > RESIDENT_BUDGET_CHARS){
            const { from, to } = this.view.viewport;
            const mid = (from + to) / 2;
            const { segs } = (0,gapTable/* gapTableOf */.kl)(this.view.state);
            let victim = -1;
            let far = -1;
            segs.forEach((s, i)=>{
                const away = Math.abs((s.from + s.to) / 2 - mid);
                const visible = s.from <= to && s.to >= from;
                if (s.kind === "loaded" && !visible && away > far && !isDirtyRange(track, s.from, s.to)) [victim, far] = [
                    i,
                    away
                ];
            });
            if (victim < 0 || !evictFromLoaded(this.view, victim, 0, segs[victim].lines)) return;
        }
    }
    reveal(line, col) {
        const table = (0,gapTable/* gapTableOf */.kl)(this.view.state);
        const hit = (0,gapTable/* segAtFileLine */.fm)(table, Math.max(0, Math.floor(line) - 1));
        const last = table.prefix[table.segs.length - 1] + table.segs[table.segs.length - 1].lines;
        if (!hit) return this.reveal(Math.min(Math.max(1, line), last), col);
        this.revealTo = {
            line: Math.floor(line),
            col
        };
        if (table.segs[hit.index].kind === "loaded") return this.settleReveal();
        const seg = table.segs[hit.index];
        const top = this.view.lineBlockAt(seg.from).top;
        this.view.scrollDOM.scrollTop = top + hit.offset * gapTable/* LINE_HEIGHT */.$g;
        this.onDemand([
            {
                index: hit.index,
                offset: hit.offset
            }
        ]);
    }
    /** Completes a pending reveal once its line is loaded: cursor there and centered. */ settleReveal() {
        const want = this.revealTo;
        if (!want) return;
        const table = (0,gapTable/* gapTableOf */.kl)(this.view.state);
        const hit = (0,gapTable/* segAtFileLine */.fm)(table, want.line - 1);
        if (!hit || table.segs[hit.index].kind !== "loaded") return;
        this.revealTo = null;
        const doc = this.view.state.doc;
        const target = doc.line(doc.lineAt(table.segs[hit.index].from).number + hit.offset);
        const pos = Math.min(target.from + Math.max(0, want.col), target.to);
        this.view.dispatch({
            selection: {
                anchor: pos
            },
            effects: dist/* EditorView */.Lz.scrollIntoView(pos, {
                y: "center"
            })
        });
    }
    viewport() {
        const { from, to } = this.view.viewport;
        const doc = this.view.state.doc;
        return {
            startLine: (0,gapTable/* absLineOfDocLine */.yi)(this.view.state, doc.lineAt(from).number),
            endLine: (0,gapTable/* absLineOfDocLine */.yi)(this.view.state, doc.lineAt(to).number)
        };
    }
    cursorLine() {
        const head = this.view.state.selection.main.head;
        return (0,gapTable/* absLineOfDocLine */.yi)(this.view.state, this.view.state.doc.lineAt(head).number);
    }
    destroy() {
        this.attach(null, null);
    }
    constructor(view){
        this.view = view;
        this.mode = "gap-line";
        this.track = null;
        this.sink = null;
        this.needCb = null;
        this.fetcher = null;
        this.asked = new Map();
        this.revealTo = null;
    }
}

;// ./src/components/sessions/files/engine/cm6LazyDoc/gutter.ts


/** Line numbers read through the gap table, so line 401 after a loaded window shows 401. */ const absoluteLineNumbers = ()=>(0,dist/* lineNumbers */.$K)({
        formatNumber: (n, state)=>String((0,gapTable/* absLineOfDocLine */.yi)(state, n))
    });

;// ./src/components/sessions/files/engine/cm6LazyDoc/guards.ts


/** True when any change overlaps a placeholder line or the newline joining it to a neighbor. */ function touchesPlaceholder(table, tr) {
    let bad = false;
    tr.changes.iterChangedRanges((fromA, toA)=>{
        for (const s of table.segs){
            if (s.kind !== "gap") continue;
            const start = Math.max(0, s.from - 1);
            const end = s.to + 1;
            const hit = toA > fromA ? fromA < end && toA > start : fromA > start && fromA < end;
            if (hit) bad = true;
        }
    });
    return bad;
}
/** Refuses user edits into placeholders; load and evict transactions are the only writers there. */ const placeholderGuard = state_dist/* EditorState */.$t.changeFilter.of((tr)=>(0,gapTable/* isLazyLoad */.xf)(tr) || !touchesPlaceholder((0,gapTable/* gapTableOf */.kl)(tr.startState), tr));

;// ./src/components/sessions/files/engine/cm6LazyDoc/placeholderWidget.ts



class GapWidget extends dist/* WidgetType */.xO {
    eq(other) {
        return other.lines === this.lines;
    }
    /** Proportional scrollbar: a gap is as tall as the lines it stands for. */ get estimatedHeight() {
        return this.lines * gapTable/* LINE_HEIGHT */.$g;
    }
    toDOM() {
        const el = document.createElement("div");
        el.className = "cm-lazy-gap";
        el.setAttribute("aria-hidden", "true");
        el.style.height = "".concat(this.lines * gapTable/* LINE_HEIGHT */.$g, "px");
        el.textContent = "… ".concat(this.lines, " lines not loaded");
        return el;
    }
    ignoreEvent() {
        return true;
    }
    constructor(lines){
        super(), this.lines = lines;
    }
}
function placeholders(state) {
    const ranges = (0,gapTable/* gapTableOf */.kl)(state).segs.filter((s)=>s.kind === "gap").map((s)=>dist/* Decoration */.NZ.replace({
            widget: new GapWidget(s.lines),
            block: true
        }).range(s.from, s.to));
    return dist/* Decoration */.NZ.set(ranges, true);
}
/** Block-replace widgets for the gaps; atomic so the cursor skips over them. */ const placeholderField = state_dist/* StateField */.sU.define({
    create: placeholders,
    update: (_value, tr)=>placeholders(tr.state),
    provide: (f)=>[
            dist/* EditorView */.Lz.decorations.from(f),
            dist/* EditorView */.Lz.atomicRanges.of((view)=>view.state.field(f))
        ]
});

;// ./src/components/sessions/files/engine/cm6LazyDoc/scrollAnchor.ts
// Viewport-driven demand for placeholders. CodeMirror keeps the scroll anchor itself when a load above the
// viewport changes heights (verified by the files-lazydoc-spike regression); this plugin only decides what to ask for.



const DEBOUNCE_MS = 30;
const demandSink = state_dist/* Facet */.sj.define({
    combine: (v)=>{
        var _v_;
        return (_v_ = v[0]) !== null && _v_ !== void 0 ? _v_ : null;
    }
});
function visibleGaps(view) {
    const rect = view.scrollDOM.getBoundingClientRect();
    if (view.scrollDOM.clientHeight === 0) return []; // no layout (jsdom, hidden tab): nothing is visible
    const top = rect.top - view.documentTop;
    const table = (0,gapTable/* gapTableOf */.kl)(view.state);
    const found = new Map();
    for (const y of [
        top,
        top + rect.height - 1
    ]){
        const block = view.lineBlockAtHeight(y);
        const index = (0,gapTable/* segIndexAtPos */.l7)(table, block.from);
        if (table.segs[index].kind !== "gap") continue;
        const offset = Math.min(table.segs[index].lines - 1, Math.max(0, Math.floor((y - block.top) / gapTable/* LINE_HEIGHT */.$g)));
        found.set("".concat(index, ":").concat(offset), {
            index,
            offset
        });
    }
    return [
        ...found.values()
    ];
}
const demandPlugin = dist/* ViewPlugin */.Z9.fromClass(class {
    update(u) {
        if (u.docChanged || u.viewportChanged || u.geometryChanged) this.schedule();
    }
    schedule() {
        if (this.timer !== null) clearTimeout(this.timer);
        this.timer = setTimeout(()=>{
            var _this_view_state_facet_target, _this_view_state_facet;
            this.timer = null;
            const demand = visibleGaps(this.view);
            if (demand.length) (_this_view_state_facet = this.view.state.facet(demandSink)) === null || _this_view_state_facet === void 0 ? void 0 : (_this_view_state_facet_target = _this_view_state_facet.target) === null || _this_view_state_facet_target === void 0 ? void 0 : _this_view_state_facet_target.call(_this_view_state_facet, demand);
        }, DEBOUNCE_MS);
    }
    destroy() {
        if (this.timer !== null) clearTimeout(this.timer);
    }
    constructor(view){
        this.view = view;
        this.timer = null;
        this.schedule();
    }
});

;// ./src/components/sessions/files/engine/cm6LazyDoc/extension.ts






/** Everything a gap-line buffer's EditorState carries; the document itself is `initialDocFor(segs)`. */ function gapLineExtension(sink, segs) {
    return [
        gapTable/* initialSegs */.pM.of(segs),
        gapTable/* gapTableField */.Ft,
        placeholderField,
        absoluteLineNumbers(),
        placeholderGuard,
        demandSink.of(sink),
        demandPlugin,
        // Fixed line height keeps the placeholder's estimated height exact and scroll anchoring stable.
        dist/* EditorView */.Lz.theme({
            ".cm-line": {
                height: "".concat(gapTable/* LINE_HEIGHT */.$g, "px"),
                lineHeight: "".concat(gapTable/* LINE_HEIGHT */.$g, "px")
            }
        })
    ];
}

;// ./src/components/sessions/files/engine/cm6LazyDoc/windowSwap.ts
// Window-swap model (ADR-036 a1, Task 1.6.4d): one window of the file per EditorState; reaching a window edge
// swaps in a new state centred on the viewport. Not the default: selected through CmEngine's `lazyMode` only
// if the owner phone gate records gap-line (a2) FAIL. A window with unsaved edits is pinned, never swapped.





const WINDOW_LINES = 2000;
const EDGE_LINES = 200;
const EDGE_CHECK_MS = 50;
/** 0-based file line of the window's first line. */ const windowStart = state_dist/* Facet */.sj.define({
    combine: (v)=>{
        var _v_;
        return (_v_ = v[0]) !== null && _v_ !== void 0 ? _v_ : 0;
    }
});
const windowExtension = (start)=>[
        windowStart.of(start),
        (0,dist/* lineNumbers */.$K)({
            formatNumber: (n, state)=>String(state.facet(windowStart) + n)
        }),
        dist/* EditorView */.Lz.theme({
            ".cm-line": {
                height: "".concat(gapTable/* LINE_HEIGHT */.$g, "px"),
                lineHeight: "".concat(gapTable/* LINE_HEIGHT */.$g, "px")
            }
        })
    ];
class WindowSwapController {
    get start() {
        return this.view.state.facet(windowStart);
    }
    attach(track, info) {
        this.track = track;
        this.info = info;
        this.asked = null;
        this.revealTo = null;
        if (info) setTimeout(()=>this.loaded() || this.requestAround(1), 0);
    }
    onNeedRange(cb) {
        this.needCb = cb;
    }
    setFetcher(fetcher) {
        this.fetcher = fetcher;
    }
    absorb(_tr) {
        return false;
    }
    isDirty() {
        return this.track !== null && unacked(this.track) !== null;
    }
    /** 1-based file line at the top of the viewport. */ topLine() {
        const block = this.view.lineBlockAtHeight(this.view.scrollDOM.getBoundingClientRect().top - this.view.documentTop);
        return this.start + this.view.state.doc.lineAt(block.from).number;
    }
    requestAround(line) {
        var _this_info, _this_fetcher, _this;
        var _this_info_lineCount;
        const total = (_this_info_lineCount = (_this_info = this.info) === null || _this_info === void 0 ? void 0 : _this_info.lineCount) !== null && _this_info_lineCount !== void 0 ? _this_info_lineCount : 0;
        const first = Math.max(0, Math.min(Math.max(0, total - WINDOW_LINES), line - 1 - WINDOW_LINES / 2));
        const range = {
            startLine: first + 1,
            endLine: Math.min(total, first + WINDOW_LINES)
        };
        const key = "".concat(range.startLine, "-").concat(range.endLine);
        if (this.asked === key) return;
        this.asked = key;
        if (this.needCb) this.needCb(range);
        else void ((_this_fetcher = (_this = this).fetcher) === null || _this_fetcher === void 0 ? void 0 : _this_fetcher.call(_this, range).then((text)=>this.provideRange(range, text), ()=>this.asked = null));
    }
    checkEdges() {
        if (!this.info || this.isDirty() || this.view.scrollDOM.clientHeight === 0) return;
        const top = this.topLine();
        const rel = top - this.start;
        const total = this.info.lineCount;
        if (rel < EDGE_LINES && this.start > 0 || rel > WINDOW_LINES - EDGE_LINES && this.start + WINDOW_LINES < total) {
            this.requestAround(top);
        }
    }
    provideRange(range, text) {
        var _this_revealTo;
        const { track, info } = this;
        if (!track || !(info === null || info === void 0 ? void 0 : info.makeState)) throw new Error("CmEngine: provideRange needs a lazy source");
        this.asked = null;
        if (this.isDirty()) return; // pinned: history cannot follow the document to a new state
        const anchor = this.loaded() ? this.topLine() : range.startLine;
        const state = info.makeState(text, range.startLine - 1);
        this.view.setState(state);
        track.base = state.doc;
        track.pending = null;
        track.inflight = null;
        var _this_revealTo_line;
        const line = (_this_revealTo_line = (_this_revealTo = this.revealTo) === null || _this_revealTo === void 0 ? void 0 : _this_revealTo.line) !== null && _this_revealTo_line !== void 0 ? _this_revealTo_line : anchor;
        const rel = Math.max(0, line - range.startLine);
        // setState resets CodeMirror's scroll anchor on its next measure, so the offset is written after that.
        this.view.requestMeasure({
            read: ()=>null,
            write: ()=>{
                const scroller = this.view.scrollDOM;
                scroller.scrollTop = rel * gapTable/* LINE_HEIGHT */.$g;
                requestAnimationFrame(()=>scroller.scrollTop = rel * gapTable/* LINE_HEIGHT */.$g);
            }
        });
        this.settleReveal();
    }
    evictRange() {
    // Nothing to evict: a window is replaced as a whole.
    }
    edits() {
        const changes = this.track ? unacked(this.track) : null;
        if (!changes || !this.track) return [];
        const start = this.start;
        return changesToEdits(changes, this.track.base).map((e)=>({
                ...e,
                start: {
                    ...e.start,
                    line: e.start.line + start
                },
                end: {
                    ...e.end,
                    line: e.end.line + start
                }
            }));
    }
    reveal(line, col) {
        this.revealTo = {
            line: Math.floor(line),
            col
        };
        if (!this.settleReveal()) this.requestAround(Math.floor(line));
    }
    settleReveal() {
        const want = this.revealTo;
        const rel = want ? want.line - this.start : 0;
        const doc = this.view.state.doc;
        if (!want || rel < 1 || rel > doc.lines) return false;
        this.revealTo = null;
        const target = doc.line(rel);
        const pos = Math.min(target.from + Math.max(0, want.col), target.to);
        this.view.dispatch({
            selection: {
                anchor: pos
            },
            effects: dist/* EditorView */.Lz.scrollIntoView(pos, {
                y: "center"
            })
        });
        return true;
    }
    viewport() {
        const { from, to } = this.view.viewport;
        const doc = this.view.state.doc;
        return {
            startLine: this.start + doc.lineAt(from).number,
            endLine: this.start + doc.lineAt(to).number
        };
    }
    cursorLine() {
        return this.start + this.view.state.doc.lineAt(this.view.state.selection.main.head).number;
    }
    destroy() {
        this.view.scrollDOM.removeEventListener("scroll", this.onScroll);
        if (this.timer !== null) clearTimeout(this.timer);
    }
    constructor(view){
        this.view = view;
        this.mode = "window-swap";
        this.track = null;
        this.info = null;
        this.needCb = null;
        this.fetcher = null;
        this.asked = null;
        this.timer = null;
        this.revealTo = null;
        this.onScroll = ()=>{
            if (this.timer === null) this.timer = setTimeout(()=>(this.timer = null, this.checkEdges()), EDGE_CHECK_MS);
        };
        this.loaded = ()=>this.view.state.doc.length > 0 || this.start > 0;
        view.scrollDOM.addEventListener("scroll", this.onScroll, {
            passive: true
        });
    }
}

;// ./src/components/sessions/files/engine/cm6LazyDoc/lazyBuffer.ts




/** Document and extensions for a lazy buffer of `lineCount` lines; `base` is the engine's own extension set. */ function lazyBufferParts(mode, lineCount, base) {
    const sink = {
        target: null
    };
    const info = {
        mode,
        lineCount,
        sink
    };
    if (mode === "window-swap") {
        info.makeState = (doc, start)=>state_dist/* EditorState */.$t.create({
                doc,
                extensions: [
                    ...base,
                    windowExtension(start)
                ]
            });
        return {
            doc: "",
            extensions: [
                ...base,
                windowExtension(0)
            ],
            info
        };
    }
    const segs = (0,gapTable/* splitGap */.b1)(lineCount);
    return {
        doc: (0,gapTable/* initialDocFor */.I2)(segs),
        extensions: [
            ...base,
            gapLineExtension(sink, segs)
        ],
        info
    };
}

;// ./src/components/sessions/files/engine/CmEngine.ts

















const GUTTER_MARKER_CLASS = {
    add: CodeEditor_css/* gutterMarkerAdd */.d9,
    delete: CodeEditor_css/* gutterMarkerDelete */.Lw,
    modify: CodeEditor_css/* gutterMarkerModify */.eD
};
class ChangeGutterMarker extends dist/* GutterMarker */.wJ {
    eq(other) {
        return other.markType === this.markType;
    }
    toDOM() {
        const el = document.createElement("div");
        el.className = GUTTER_MARKER_CLASS[this.markType];
        return el;
    }
    constructor(markType){
        super(), this.markType = markType;
    }
}
class CmScroll {
    constructor(effect){
        this.effect = effect;
        this.kind = "scroll-snapshot";
    }
}
/** One open file: its EditorState plus the edit bookkeeping that must travel with it across tab swaps. */ class CmBuffer {
    constructor(state){
        this.state = state;
        this.kind = "buffer-state";
        this.pending = null;
        this.inflight = null;
        this./** Set for a buffer whose document is only partly loaded (cm6LazyDoc). */ lazy = null;
        this.base = state.doc;
    }
}
const FEATURES = {
    vim: false,
    mergeView: false,
    lazyDoc: true,
    irc: true
};
function modeExtensions(c) {
    var _c_mode;
    const readOnly = ((_c_mode = c.mode) !== null && _c_mode !== void 0 ? _c_mode : "Read") === "Read";
    var _c_ariaLabel;
    return {
        readOnly: [
            readOnlyExtension(readOnly),
            dist/* EditorView */.Lz.contentAttributes.of({
                "aria-readonly": String(readOnly)
            })
        ],
        wrap: c.wrap ? [
            dist/* EditorView */.Lz.lineWrapping
        ] : [],
        // Explicit even though CM6 defaults them off: guards against a default change and soft-keyboard smart punctuation.
        label: dist/* EditorView */.Lz.contentAttributes.of({
            "aria-label": (_c_ariaLabel = c.ariaLabel) !== null && _c_ariaLabel !== void 0 ? _c_ariaLabel : "File",
            autocorrect: "off",
            autocapitalize: "off",
            spellcheck: "false",
            // Read mode on a touch device keeps the text selectable but never raises the keyboard.
            ...readOnly && c.noVirtualKeyboard ? {
                inputmode: "none"
            } : {}
        }),
        font: c.fontPx ? dist/* EditorView */.Lz.theme({
            "&": {
                fontSize: "".concat(c.fontPx, "px")
            }
        }) : []
    };
}
class CmEngine {
    makeController(view) {
        return this.lazyMode === "window-swap" ? new WindowSwapController(view) : new GapLineController(view);
    }
    async createBuffer(init) {
        var _init_config;
        const degrade = (_init_config = init.config) === null || _init_config === void 0 ? void 0 : _init_config.degrade;
        let lang = null;
        try {
            var _init_language;
            if ((degrade === null || degrade === void 0 ? void 0 : degrade.syntaxHighlight) !== false) lang = await loadCodemirrorLang((_init_language = init.language) !== null && _init_language !== void 0 ? _init_language : "text");
        } catch (e) {
        // Unsupported language: plain text.
        }
        const marks = init.gutterMarks;
        const changeGutter = (0,dist/* gutter */.cU)({
            class: "cm-changeGutter",
            lineMarker (view, block) {
                if (!marks || marks.size === 0) return null;
                const markType = markAtDocLine(view.state, marks, view.state.doc.lineAt(block.from).number);
                return markType ? new ChangeGutterMarker(markType) : null;
            }
        });
        var _init_config1;
        // Wrapped lines break the fixed line height a lazy document's placeholders are sized with.
        const mode = modeExtensions(init.lazy ? {
            ...init.config,
            wrap: false
        } : (_init_config1 = init.config) !== null && _init_config1 !== void 0 ? _init_config1 : {});
        // view.setState drops every extension, so each buffer state carries the full set itself.
        const extensions = [
            setupFor(degrade),
            cmTheme,
            legacyVimKeymap(),
            changeGutter,
            diffHighlightExtension(marks),
            this.comps.readOnly.of(mode.readOnly),
            this.comps.wrap.of(mode.wrap),
            this.comps.label.of(mode.label),
            this.comps.font.of(mode.font),
            degradeExtension(degrade),
            searchHighlightExtension(),
            vimExtension(),
            mergeExtension()
        ];
        if (lang) extensions.push(lang);
        // The server counts lines by "\n" only; CodeMirror would also split a lone CR and drift from the gap table.
        if (init.lazy) extensions.push(state_dist/* EditorState */.$t.lineSeparator.of("\n"));
        if (!init.lazy) return new CmBuffer(state_dist/* EditorState */.$t.create({
            doc: init.doc,
            extensions
        }));
        const parts = lazyBufferParts(this.lazyMode, init.lazy.lineCount, extensions);
        const buffer = new CmBuffer(state_dist/* EditorState */.$t.create({
            doc: parts.doc,
            extensions: parts.extensions
        }));
        buffer.lazy = parts.info;
        return buffer;
    }
    mount(el, options) {
        return new CmHandle(this, el, options);
    }
    reconfigureEffects(config) {
        const m = modeExtensions(config);
        return [
            this.comps.readOnly.reconfigure(m.readOnly),
            this.comps.wrap.reconfigure(m.wrap),
            this.comps.label.reconfigure(m.label),
            this.comps.font.reconfigure(m.font)
        ];
    }
    /** `lazyMode` picks the lazy-document model; gap-line (a2) is the default, window-swap (a1) the unselected fallback. */ constructor(lazyMode = "gap-line"){
        this.lazyMode = lazyMode;
        this.features = FEATURES;
        this.comps = {
            readOnly: new state_dist/* Compartment */.xx(),
            wrap: new state_dist/* Compartment */.xx(),
            label: new state_dist/* Compartment */.xx(),
            font: new state_dist/* Compartment */.xx()
        };
    }
}
class CmHandle {
    /** Points the lazy controller (created on first use) at the current buffer, or at nothing for a fully loaded one. */ bindLazy() {
        var _this_lazy;
        const info = this.buffer.lazy;
        if (info && !this.lazy) {
            this.lazy = this.engine.makeController(this.view);
            if (this.needCb) this.lazy.onNeedRange(this.needCb);
        }
        (_this_lazy = this.lazy) === null || _this_lazy === void 0 ? void 0 : _this_lazy.attach(info ? this.buffer : null, info);
    }
    lazyOrThrow() {
        if (!this.buffer.lazy || !this.lazy) throw new Error("CmEngine: lazyDoc source required (setSource with a lineCount)");
        return this.lazy;
    }
    recordChange(changes) {
        var _this_editsCb, _this;
        const b = this.buffer;
        b.pending = b.pending ? b.pending.compose(changes) : changes;
        (_this_editsCb = (_this = this).editsCb) === null || _this_editsCb === void 0 ? void 0 : _this_editsCb.call(_this, {
            edits: this.pendingEdits()
        });
    }
    pendingEdits() {
        const b = this.buffer;
        var _b_inflight;
        const all = b.inflight && b.pending ? b.inflight.compose(b.pending) : (_b_inflight = b.inflight) !== null && _b_inflight !== void 0 ? _b_inflight : b.pending;
        if (!all) return [];
        return this.buffer.lazy && this.lazy ? this.lazy.edits() : changesToEdits(all, b.base);
    }
    /** CM6 state must not change under an active IME composition (ADR-032): defer until it ends. */ whenIdle(fn) {
        if (this.view.compositionStarted) this.queued.push(fn);
        else fn();
    }
    flushQueued() {
        if (this.view.compositionStarted) return;
        this.queued.splice(0).forEach((fn)=>fn());
    }
    createBuffer(init) {
        var _init_config;
        return this.engine.createBuffer({
            ...init,
            config: (_init_config = init.config) !== null && _init_config !== void 0 ? _init_config : this.config
        });
    }
    getBuffer() {
        this.buffer.state = this.view.state;
        return this.buffer;
    }
    setBuffer(next) {
        this.whenIdle(()=>{
            this.buffer.state = this.view.state;
            this.buffer = next;
            this.view.setState(this.buffer.state);
            this.bindLazy();
            this.view.dispatch({
                effects: this.engine.reconfigureEffects(this.effectiveConfig())
            });
        });
    }
    setSource(src) {
        if (src.text === undefined && src.lineCount === undefined) throw new Error("CmEngine: setSource needs the full text or a lineCount");
        var _src_lineCount;
        const lazy = src.text === undefined ? {
            lineCount: (_src_lineCount = src.lineCount) !== null && _src_lineCount !== void 0 ? _src_lineCount : 0
        } : undefined;
        var _src_text;
        void this.createBuffer({
            doc: (_src_text = src.text) !== null && _src_text !== void 0 ? _src_text : "",
            lazy
        }).then((b)=>{
            this.setBuffer(b);
            this.whenIdle(()=>{
                var _src_fetch, _this_lazy;
                const fetchRange = (_src_fetch = src.fetch) === null || _src_fetch === void 0 ? void 0 : _src_fetch.bind(src);
                (_this_lazy = this.lazy) === null || _this_lazy === void 0 ? void 0 : _this_lazy.setFetcher(fetchRange !== null && fetchRange !== void 0 ? fetchRange : null);
            });
        });
    }
    onNeedRange(cb) {
        var _this_lazy;
        this.needCb = cb;
        (_this_lazy = this.lazy) === null || _this_lazy === void 0 ? void 0 : _this_lazy.onNeedRange(cb);
    }
    provideRange(range, text) {
        const lazy = this.lazyOrThrow();
        this.whenIdle(()=>lazy.provideRange(range, text));
    }
    evictRange(range) {
        const lazy = this.lazyOrThrow();
        this.whenIdle(()=>lazy.evictRange(range));
    }
    onEdits(cb) {
        this.editsCb = cb;
    }
    takePending() {
        const b = this.buffer;
        if (b.pending) {
            b.inflight = b.inflight ? b.inflight.compose(b.pending) : b.pending;
            b.pending = null;
        }
        return {
            edits: this.pendingEdits()
        };
    }
    ack(_newToken) {
        const b = this.buffer;
        if (b.inflight) b.base = b.inflight.apply(b.base);
        b.inflight = null;
    }
    getText() {
        return this.view.state.doc.toString();
    }
    getCursor() {
        const head = this.view.state.selection.main.head;
        const line = this.view.state.doc.lineAt(head);
        return {
            line: this.buffer.lazy && this.lazy ? this.lazy.cursorLine() : line.number,
            col: head - line.from
        };
    }
    insertText(text) {
        if (this.view.state.readOnly || this.view.compositionStarted) return false;
        this.view.dispatch(this.view.state.replaceSelection(text), {
            userEvent: "input.type",
            scrollIntoView: true
        });
        return true;
    }
    reveal(line) {
        let col = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : 0;
        if (this.buffer.lazy && this.lazy) return this.lazy.reveal(line, col);
        const doc = this.view.state.doc;
        const target = doc.line(Math.min(Math.max(1, Math.floor(line)), doc.lines));
        const pos = Math.min(target.from + Math.max(0, col), target.to);
        this.view.dispatch({
            selection: {
                anchor: pos
            },
            effects: dist/* EditorView */.Lz.scrollIntoView(pos, {
                y: "center"
            })
        });
    }
    highlight(range) {
        this.view.dispatch({
            effects: setSearchHit.of(range && this.docLineOf(range))
        });
    }
    /** A gap-line document holds only loaded lines, so a hit's file line is mapped to its document line (null while unloaded). */ docLineOf(range) {
        var _this_lazy;
        if (((_this_lazy = this.lazy) === null || _this_lazy === void 0 ? void 0 : _this_lazy.mode) !== "gap-line" || !this.buffer.lazy) return range;
        const table = (0,gapTable/* gapTableOf */.kl)(this.view.state);
        const at = (0,gapTable/* segAtFileLine */.fm)(table, range.line - 1);
        const seg = at && table.segs[at.index];
        if (!at || (seg === null || seg === void 0 ? void 0 : seg.kind) !== "loaded") return null;
        return {
            ...range,
            line: this.view.state.doc.lineAt(seg.from).number + at.offset
        };
    }
    getViewport() {
        if (this.buffer.lazy && this.lazy) return this.lazy.viewport();
        const { from, to } = this.view.viewport;
        const doc = this.view.state.doc;
        return {
            startLine: doc.lineAt(from).number,
            endLine: doc.lineAt(to).number
        };
    }
    scrollSnapshot() {
        return new CmScroll(this.view.scrollSnapshot());
    }
    restoreScroll(snapshot) {
        this.view.dispatch({
            effects: snapshot.effect
        });
    }
    setReadOnly(readOnly) {
        this.setMode({
            mode: readOnly ? "Read" : "Edit"
        });
    }
    setMode(config) {
        this.config = {
            ...this.config,
            ...config
        };
        this.whenIdle(()=>this.view.dispatch({
                effects: this.engine.reconfigureEffects(this.effectiveConfig())
            }));
    }
    effectiveConfig() {
        return this.buffer.lazy ? {
            ...this.config,
            wrap: false
        } : this.config;
    }
    focus() {
        this.view.focus();
    }
    destroy() {
        var _this_lazy;
        this.el.removeEventListener("compositionend", this.onCompositionEnd);
        (_this_lazy = this.lazy) === null || _this_lazy === void 0 ? void 0 : _this_lazy.destroy();
        this.view.destroy();
    }
    constructor(engine, el, options){
        this.engine = engine;
        this.el = el;
        this.features = FEATURES;
        this.editsCb = null;
        this.queued = [];
        this.lazy = null;
        this.needCb = null;
        this.onCompositionEnd = ()=>setTimeout(()=>this.flushQueued(), 0);
        this.buffer = options.buffer;
        this.config = {
            mode: options.mode,
            wrap: options.wrap,
            ariaLabel: options.ariaLabel,
            fontPx: options.fontPx,
            noVirtualKeyboard: options.noVirtualKeyboard
        };
        this.view = new dist/* EditorView */.Lz({
            state: this.buffer.state,
            parent: el,
            dispatchTransactions: (trs, view)=>{
                var _this_lazy;
                view.update(trs);
                for (const tr of trs)if (tr.docChanged && !((_this_lazy = this.lazy) === null || _this_lazy === void 0 ? void 0 : _this_lazy.absorb(tr))) this.recordChange(tr.changes);
            }
        });
        this.bindLazy();
        // compositionend bubbles from contentDOM; listening on the host runs after CM has cleared its own state.
        el.addEventListener("compositionend", this.onCompositionEnd);
    }
}


/***/ })

}]);
//# sourceMappingURL=2086-3ad60ede29f11d41.js.map