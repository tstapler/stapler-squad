"use strict";
exports.id = 8513;
exports.ids = [8513];
exports.modules = {

/***/ 28513:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  oQ: () => (/* binding */ basicSetup),
  V0: () => (/* binding */ minimalSetup)
});

// UNUSED EXPORTS: EditorView

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+view@6.41.0/node_modules/@codemirror/view/dist/index.js + 1 modules
var dist = __webpack_require__(87748);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+state@6.6.0/node_modules/@codemirror/state/dist/index.js + 1 modules
var state_dist = __webpack_require__(64614);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+language@6.12.3/node_modules/@codemirror/language/dist/index.js
var language_dist = __webpack_require__(35088);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+commands@6.10.3/node_modules/@codemirror/commands/dist/index.js
var commands_dist = __webpack_require__(19154);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+search@6.6.0/node_modules/@codemirror/search/dist/index.js
var search_dist = __webpack_require__(93756);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+autocomplete@6.20.1/node_modules/@codemirror/autocomplete/dist/index.js
var autocomplete_dist = __webpack_require__(38605);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/crelt@1.0.6/node_modules/crelt/index.js
var crelt = __webpack_require__(59984);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+lint@6.9.5/node_modules/@codemirror/lint/dist/index.js




class SelectedDiagnostic {
    constructor(from, to, diagnostic) {
        this.from = from;
        this.to = to;
        this.diagnostic = diagnostic;
    }
}
class LintState {
    constructor(diagnostics, panel, selected) {
        this.diagnostics = diagnostics;
        this.panel = panel;
        this.selected = selected;
    }
    static init(diagnostics, panel, state) {
        // Filter the list of diagnostics for which to create markers
        let diagnosticFilter = state.facet(lintConfig).markerFilter;
        if (diagnosticFilter)
            diagnostics = diagnosticFilter(diagnostics, state);
        let sorted = diagnostics.slice().sort((a, b) => a.from - b.from || a.to - b.to);
        let deco = new state_dist/* RangeSetBuilder */.vB(), active = [], pos = 0;
        let scan = state.doc.iter(), scanPos = 0, docLen = state.doc.length;
        for (let i = 0;;) {
            let next = i == sorted.length ? null : sorted[i];
            if (!next && !active.length)
                break;
            let from, to;
            if (active.length) {
                from = pos;
                to = active.reduce((p, d) => Math.min(p, d.to), next && next.from > from ? next.from : 1e8);
            }
            else {
                from = next.from;
                if (from > docLen)
                    break;
                to = next.to;
                active.push(next);
                i++;
            }
            while (i < sorted.length) {
                let next = sorted[i];
                if (next.from == from && (next.to > next.from || next.to == from)) {
                    active.push(next);
                    i++;
                    to = Math.min(next.to, to);
                }
                else {
                    to = Math.min(next.from, to);
                    break;
                }
            }
            to = Math.min(to, docLen);
            let widget = false;
            if (active.some(d => d.from == from && (d.to == to || to == docLen))) {
                widget = from == to;
                if (!widget && to - from < 10) {
                    let behind = from - (scanPos + scan.value.length);
                    if (behind > 0) {
                        scan.next(behind);
                        scanPos = from;
                    }
                    for (let check = from;;) {
                        if (check >= to) {
                            widget = true;
                            break;
                        }
                        if (!scan.lineBreak && scanPos + scan.value.length > check)
                            break;
                        check = scanPos + scan.value.length;
                        scanPos += scan.value.length;
                        scan.next();
                    }
                }
            }
            let sev = maxSeverity(active);
            if (widget) {
                deco.add(from, from, dist/* Decoration */.NZ.widget({
                    widget: new DiagnosticWidget(sev),
                    diagnostics: active.slice()
                }));
            }
            else {
                let markClass = active.reduce((c, d) => d.markClass ? c + " " + d.markClass : c, "");
                deco.add(from, to, dist/* Decoration */.NZ.mark({
                    class: "cm-lintRange cm-lintRange-" + sev + markClass,
                    diagnostics: active.slice(),
                    inclusiveEnd: active.some(a => a.to > to)
                }));
            }
            pos = to;
            if (pos == docLen)
                break;
            for (let i = 0; i < active.length; i++)
                if (active[i].to <= pos)
                    active.splice(i--, 1);
        }
        let set = deco.finish();
        return new LintState(set, panel, findDiagnostic(set));
    }
}
function findDiagnostic(diagnostics, diagnostic = null, after = 0) {
    let found = null;
    diagnostics.between(after, 1e9, (from, to, { spec }) => {
        if (diagnostic && spec.diagnostics.indexOf(diagnostic) < 0)
            return;
        if (!found)
            found = new SelectedDiagnostic(from, to, diagnostic || spec.diagnostics[0]);
        else if (spec.diagnostics.indexOf(found.diagnostic) < 0)
            return false;
        else
            found = new SelectedDiagnostic(found.from, to, found.diagnostic);
    });
    return found;
}
function hideTooltip(tr, tooltip) {
    let from = tooltip.pos, to = tooltip.end || from;
    let result = tr.state.facet(lintConfig).hideOn(tr, from, to);
    if (result != null)
        return result;
    let line = tr.startState.doc.lineAt(tooltip.pos);
    return !!(tr.effects.some(e => e.is(setDiagnosticsEffect)) || tr.changes.touchesRange(line.from, Math.max(line.to, to)));
}
function maybeEnableLint(state, effects) {
    return state.field(lintState, false) ? effects : effects.concat(state_dist/* StateEffect */.Pe.appendConfig.of(lintExtensions));
}
/**
Returns a transaction spec which updates the current set of
diagnostics, and enables the lint extension if if wasn't already
active.
*/
function setDiagnostics(state, diagnostics) {
    return {
        effects: maybeEnableLint(state, [setDiagnosticsEffect.of(diagnostics)])
    };
}
/**
The state effect that updates the set of active diagnostics. Can
be useful when writing an extension that needs to track these.
*/
const setDiagnosticsEffect = /*@__PURE__*/state_dist/* StateEffect */.Pe.define();
const togglePanel = /*@__PURE__*/state_dist/* StateEffect */.Pe.define();
const movePanelSelection = /*@__PURE__*/state_dist/* StateEffect */.Pe.define();
const lintState = /*@__PURE__*/state_dist/* StateField */.sU.define({
    create() {
        return new LintState(dist/* Decoration */.NZ.none, null, null);
    },
    update(value, tr) {
        if (tr.docChanged && value.diagnostics.size) {
            let mapped = value.diagnostics.map(tr.changes), selected = null, panel = value.panel;
            if (value.selected) {
                let selPos = tr.changes.mapPos(value.selected.from, 1);
                selected = findDiagnostic(mapped, value.selected.diagnostic, selPos) || findDiagnostic(mapped, null, selPos);
            }
            if (!mapped.size && panel && tr.state.facet(lintConfig).autoPanel)
                panel = null;
            value = new LintState(mapped, panel, selected);
        }
        for (let effect of tr.effects) {
            if (effect.is(setDiagnosticsEffect)) {
                let panel = !tr.state.facet(lintConfig).autoPanel ? value.panel : effect.value.length ? LintPanel.open : null;
                value = LintState.init(effect.value, panel, tr.state);
            }
            else if (effect.is(togglePanel)) {
                value = new LintState(value.diagnostics, effect.value ? LintPanel.open : null, value.selected);
            }
            else if (effect.is(movePanelSelection)) {
                value = new LintState(value.diagnostics, value.panel, effect.value);
            }
        }
        return value;
    },
    provide: f => [dist/* showPanel */.S7.from(f, val => val.panel),
        dist/* EditorView */.Lz.decorations.from(f, s => s.diagnostics)]
});
/**
Returns the number of active lint diagnostics in the given state.
*/
function diagnosticCount(state) {
    let lint = state.field(lintState, false);
    return lint ? lint.diagnostics.size : 0;
}
const activeMark = /*@__PURE__*/dist/* Decoration */.NZ.mark({ class: "cm-lintRange cm-lintRange-active" });
function lintTooltip(view, pos, side) {
    let { diagnostics } = view.state.field(lintState);
    let found, start = -1, end = -1;
    diagnostics.between(pos - (side < 0 ? 1 : 0), pos + (side > 0 ? 1 : 0), (from, to, { spec }) => {
        if (pos >= from && pos <= to &&
            (from == to || ((pos > from || side > 0) && (pos < to || side < 0)))) {
            found = spec.diagnostics;
            start = from;
            end = to;
            return false;
        }
    });
    let diagnosticFilter = view.state.facet(lintConfig).tooltipFilter;
    if (found && diagnosticFilter)
        found = diagnosticFilter(found, view.state);
    if (!found)
        return null;
    return {
        pos: start,
        end: end,
        above: view.state.doc.lineAt(start).to < end,
        create() {
            return { dom: diagnosticsTooltip(view, found) };
        }
    };
}
function diagnosticsTooltip(view, diagnostics) {
    return (0,crelt/* default */.A)("ul", { class: "cm-tooltip-lint" }, diagnostics.map(d => renderDiagnostic(view, d, false)));
}
/**
Command to open and focus the lint panel.
*/
const openLintPanel = (view) => {
    let field = view.state.field(lintState, false);
    if (!field || !field.panel)
        view.dispatch({ effects: maybeEnableLint(view.state, [togglePanel.of(true)]) });
    let panel = (0,dist/* getPanel */.ld)(view, LintPanel.open);
    if (panel)
        panel.dom.querySelector(".cm-panel-lint ul").focus();
    return true;
};
/**
Command to close the lint panel, when open.
*/
const closeLintPanel = (view) => {
    let field = view.state.field(lintState, false);
    if (!field || !field.panel)
        return false;
    view.dispatch({ effects: togglePanel.of(false) });
    return true;
};
/**
Move the selection to the next diagnostic.
*/
const nextDiagnostic = (view) => {
    let field = view.state.field(lintState, false);
    if (!field)
        return false;
    let sel = view.state.selection.main, next = findDiagnostic(field.diagnostics, null, sel.to + 1);
    if (!next) {
        next = findDiagnostic(field.diagnostics, null, 0);
        if (!next || next.from == sel.from && next.to == sel.to)
            return false;
    }
    view.dispatch({ selection: { anchor: next.from, head: next.to }, scrollIntoView: true });
    return true;
};
/**
Move the selection to the previous diagnostic.
*/
const previousDiagnostic = (view) => {
    let { state } = view, field = state.field(lintState, false);
    if (!field)
        return false;
    let sel = state.selection.main;
    let prevFrom, prevTo, lastFrom, lastTo;
    field.diagnostics.between(0, state.doc.length, (from, to) => {
        if (to < sel.to && (prevFrom == null || prevFrom < from)) {
            prevFrom = from;
            prevTo = to;
        }
        if (lastFrom == null || from > lastFrom) {
            lastFrom = from;
            lastTo = to;
        }
    });
    if (lastFrom == null || prevFrom == null && lastFrom == sel.from)
        return false;
    view.dispatch({ selection: { anchor: prevFrom !== null && prevFrom !== void 0 ? prevFrom : lastFrom, head: prevTo !== null && prevTo !== void 0 ? prevTo : lastTo }, scrollIntoView: true });
    return true;
};
/**
A set of default key bindings for the lint functionality.

- Ctrl-Shift-m (Cmd-Shift-m on macOS): [`openLintPanel`](https://codemirror.net/6/docs/ref/#lint.openLintPanel)
- F8: [`nextDiagnostic`](https://codemirror.net/6/docs/ref/#lint.nextDiagnostic)
*/
const lintKeymap = [
    { key: "Mod-Shift-m", run: openLintPanel, preventDefault: true },
    { key: "F8", run: nextDiagnostic }
];
const lintPlugin = /*@__PURE__*/(/* unused pure expression or super */ null && (ViewPlugin.fromClass(class {
    constructor(view) {
        this.view = view;
        this.timeout = -1;
        this.set = true;
        let { delay } = view.state.facet(lintConfig);
        this.lintTime = Date.now() + delay;
        this.run = this.run.bind(this);
        this.timeout = setTimeout(this.run, delay);
    }
    run() {
        clearTimeout(this.timeout);
        let now = Date.now();
        if (now < this.lintTime - 10) {
            this.timeout = setTimeout(this.run, this.lintTime - now);
        }
        else {
            this.set = false;
            let { state } = this.view, { sources } = state.facet(lintConfig);
            if (sources.length)
                batchResults(sources.map(s => Promise.resolve(s(this.view))), annotations => {
                    if (this.view.state.doc == state.doc)
                        this.view.dispatch(setDiagnostics(this.view.state, annotations.reduce((a, b) => a.concat(b))));
                }, error => { logException(this.view.state, error); });
        }
    }
    update(update) {
        let config = update.state.facet(lintConfig);
        if (update.docChanged || config != update.startState.facet(lintConfig) ||
            config.needsRefresh && config.needsRefresh(update)) {
            this.lintTime = Date.now() + config.delay;
            if (!this.set) {
                this.set = true;
                this.timeout = setTimeout(this.run, config.delay);
            }
        }
    }
    force() {
        if (this.set) {
            this.lintTime = Date.now();
            this.run();
        }
    }
    destroy() {
        clearTimeout(this.timeout);
    }
})));
function batchResults(promises, sink, error) {
    let collected = [], timeout = -1;
    for (let p of promises)
        p.then(value => {
            collected.push(value);
            clearTimeout(timeout);
            if (collected.length == promises.length)
                sink(collected);
            else
                timeout = setTimeout(() => sink(collected), 200);
        }, error);
}
const lintConfig = /*@__PURE__*/state_dist/* Facet */.sj.define({
    combine(input) {
        return {
            sources: input.map(i => i.source).filter(x => x != null),
            ...(0,state_dist/* combineConfig */.QR)(input.map(i => i.config), {
                delay: 750,
                markerFilter: null,
                tooltipFilter: null,
                needsRefresh: null,
                hideOn: () => null,
            }, {
                delay: Math.max,
                markerFilter: combineFilter,
                tooltipFilter: combineFilter,
                needsRefresh: (a, b) => !a ? b : !b ? a : u => a(u) || b(u),
                hideOn: (a, b) => !a ? b : !b ? a : (t, x, y) => a(t, x, y) || b(t, x, y),
                autoPanel: (a, b) => a || b
            })
        };
    }
});
function combineFilter(a, b) {
    return !a ? b : !b ? a : (d, s) => b(a(d, s), s);
}
/**
Given a diagnostic source, this function returns an extension that
enables linting with that source. It will be called whenever the
editor is idle (after its content changed).

Note that settings given here will apply to all linters active in
the editor. If `null` is given as source, this only configures the
lint extension.
*/
function linter(source, config = {}) {
    return [
        lintConfig.of({ source, config }),
        lintPlugin,
        lintExtensions
    ];
}
/**
Forces any linters [configured](https://codemirror.net/6/docs/ref/#lint.linter) to run when the
editor is idle to run right away.
*/
function forceLinting(view) {
    let plugin = view.plugin(lintPlugin);
    if (plugin)
        plugin.force();
}
function assignKeys(actions) {
    let assigned = [];
    if (actions)
        actions: for (let { name } of actions) {
            for (let i = 0; i < name.length; i++) {
                let ch = name[i];
                if (/[a-zA-Z]/.test(ch) && !assigned.some(c => c.toLowerCase() == ch.toLowerCase())) {
                    assigned.push(ch);
                    continue actions;
                }
            }
            assigned.push("");
        }
    return assigned;
}
function renderDiagnostic(view, diagnostic, inPanel) {
    var _a;
    let keys = inPanel ? assignKeys(diagnostic.actions) : [];
    return (0,crelt/* default */.A)("li", { class: "cm-diagnostic cm-diagnostic-" + diagnostic.severity }, (0,crelt/* default */.A)("span", { class: "cm-diagnosticText" }, diagnostic.renderMessage ? diagnostic.renderMessage(view) : diagnostic.message), (_a = diagnostic.actions) === null || _a === void 0 ? void 0 : _a.map((action, i) => {
        let fired = false, click = (e) => {
            e.preventDefault();
            if (fired)
                return;
            fired = true;
            let found = findDiagnostic(view.state.field(lintState).diagnostics, diagnostic);
            if (found)
                action.apply(view, found.from, found.to);
        };
        let { name } = action, keyIndex = keys[i] ? name.indexOf(keys[i]) : -1;
        let nameElt = keyIndex < 0 ? name : [name.slice(0, keyIndex),
            (0,crelt/* default */.A)("u", name.slice(keyIndex, keyIndex + 1)),
            name.slice(keyIndex + 1)];
        let markClass = action.markClass ? " " + action.markClass : "";
        return (0,crelt/* default */.A)("button", {
            type: "button",
            class: "cm-diagnosticAction" + markClass,
            onclick: click,
            onmousedown: click,
            "aria-label": ` Action: ${name}${keyIndex < 0 ? "" : ` (access key "${keys[i]})"`}.`
        }, nameElt);
    }), diagnostic.source && (0,crelt/* default */.A)("div", { class: "cm-diagnosticSource" }, diagnostic.source));
}
class DiagnosticWidget extends dist/* WidgetType */.xO {
    constructor(sev) {
        super();
        this.sev = sev;
    }
    eq(other) { return other.sev == this.sev; }
    toDOM() {
        return (0,crelt/* default */.A)("span", { class: "cm-lintPoint cm-lintPoint-" + this.sev });
    }
}
class PanelItem {
    constructor(view, diagnostic) {
        this.diagnostic = diagnostic;
        this.id = "item_" + Math.floor(Math.random() * 0xffffffff).toString(16);
        this.dom = renderDiagnostic(view, diagnostic, true);
        this.dom.id = this.id;
        this.dom.setAttribute("role", "option");
    }
}
class LintPanel {
    constructor(view) {
        this.view = view;
        this.items = [];
        let onkeydown = (event) => {
            if (event.ctrlKey || event.altKey || event.metaKey)
                return;
            if (event.keyCode == 27) { // Escape
                closeLintPanel(this.view);
                this.view.focus();
            }
            else if (event.keyCode == 38 || event.keyCode == 33) { // ArrowUp, PageUp
                this.moveSelection((this.selectedIndex - 1 + this.items.length) % this.items.length);
            }
            else if (event.keyCode == 40 || event.keyCode == 34) { // ArrowDown, PageDown
                this.moveSelection((this.selectedIndex + 1) % this.items.length);
            }
            else if (event.keyCode == 36) { // Home
                this.moveSelection(0);
            }
            else if (event.keyCode == 35) { // End
                this.moveSelection(this.items.length - 1);
            }
            else if (event.keyCode == 13) { // Enter
                this.view.focus();
            }
            else if (event.keyCode >= 65 && event.keyCode <= 90 && this.selectedIndex >= 0) { // A-Z
                let { diagnostic } = this.items[this.selectedIndex], keys = assignKeys(diagnostic.actions);
                for (let i = 0; i < keys.length; i++)
                    if (keys[i].toUpperCase().charCodeAt(0) == event.keyCode) {
                        let found = findDiagnostic(this.view.state.field(lintState).diagnostics, diagnostic);
                        if (found)
                            diagnostic.actions[i].apply(view, found.from, found.to);
                    }
            }
            else {
                return;
            }
            event.preventDefault();
        };
        let onclick = (event) => {
            for (let i = 0; i < this.items.length; i++) {
                if (this.items[i].dom.contains(event.target))
                    this.moveSelection(i);
            }
        };
        this.list = (0,crelt/* default */.A)("ul", {
            tabIndex: 0,
            role: "listbox",
            "aria-label": this.view.state.phrase("Diagnostics"),
            onkeydown,
            onclick
        });
        this.dom = (0,crelt/* default */.A)("div", { class: "cm-panel-lint" }, this.list, (0,crelt/* default */.A)("button", {
            type: "button",
            name: "close",
            "aria-label": this.view.state.phrase("close"),
            onclick: () => closeLintPanel(this.view)
        }, "×"));
        this.update();
    }
    get selectedIndex() {
        let selected = this.view.state.field(lintState).selected;
        if (!selected)
            return -1;
        for (let i = 0; i < this.items.length; i++)
            if (this.items[i].diagnostic == selected.diagnostic)
                return i;
        return -1;
    }
    update() {
        let { diagnostics, selected } = this.view.state.field(lintState);
        let i = 0, needsSync = false, newSelectedItem = null;
        let seen = new Set();
        diagnostics.between(0, this.view.state.doc.length, (_start, _end, { spec }) => {
            for (let diagnostic of spec.diagnostics) {
                if (seen.has(diagnostic))
                    continue;
                seen.add(diagnostic);
                let found = -1, item;
                for (let j = i; j < this.items.length; j++)
                    if (this.items[j].diagnostic == diagnostic) {
                        found = j;
                        break;
                    }
                if (found < 0) {
                    item = new PanelItem(this.view, diagnostic);
                    this.items.splice(i, 0, item);
                    needsSync = true;
                }
                else {
                    item = this.items[found];
                    if (found > i) {
                        this.items.splice(i, found - i);
                        needsSync = true;
                    }
                }
                if (selected && item.diagnostic == selected.diagnostic) {
                    if (!item.dom.hasAttribute("aria-selected")) {
                        item.dom.setAttribute("aria-selected", "true");
                        newSelectedItem = item;
                    }
                }
                else if (item.dom.hasAttribute("aria-selected")) {
                    item.dom.removeAttribute("aria-selected");
                }
                i++;
            }
        });
        while (i < this.items.length && !(this.items.length == 1 && this.items[0].diagnostic.from < 0)) {
            needsSync = true;
            this.items.pop();
        }
        if (this.items.length == 0) {
            this.items.push(new PanelItem(this.view, {
                from: -1, to: -1,
                severity: "info",
                message: this.view.state.phrase("No diagnostics")
            }));
            needsSync = true;
        }
        if (newSelectedItem) {
            this.list.setAttribute("aria-activedescendant", newSelectedItem.id);
            this.view.requestMeasure({
                key: this,
                read: () => ({ sel: newSelectedItem.dom.getBoundingClientRect(), panel: this.list.getBoundingClientRect() }),
                write: ({ sel, panel }) => {
                    let scaleY = panel.height / this.list.offsetHeight;
                    if (sel.top < panel.top)
                        this.list.scrollTop -= (panel.top - sel.top) / scaleY;
                    else if (sel.bottom > panel.bottom)
                        this.list.scrollTop += (sel.bottom - panel.bottom) / scaleY;
                }
            });
        }
        else if (this.selectedIndex < 0) {
            this.list.removeAttribute("aria-activedescendant");
        }
        if (needsSync)
            this.sync();
    }
    sync() {
        let domPos = this.list.firstChild;
        function rm() {
            let prev = domPos;
            domPos = prev.nextSibling;
            prev.remove();
        }
        for (let item of this.items) {
            if (item.dom.parentNode == this.list) {
                while (domPos != item.dom)
                    rm();
                domPos = item.dom.nextSibling;
            }
            else {
                this.list.insertBefore(item.dom, domPos);
            }
        }
        while (domPos)
            rm();
    }
    moveSelection(selectedIndex) {
        if (this.selectedIndex < 0)
            return;
        let field = this.view.state.field(lintState);
        let selection = findDiagnostic(field.diagnostics, this.items[selectedIndex].diagnostic);
        if (!selection)
            return;
        this.view.dispatch({
            selection: { anchor: selection.from, head: selection.to },
            scrollIntoView: true,
            effects: movePanelSelection.of(selection)
        });
    }
    static open(view) { return new LintPanel(view); }
}
function svg(content, attrs = `viewBox="0 0 40 40"`) {
    return `url('data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" ${attrs}>${encodeURIComponent(content)}</svg>')`;
}
function underline(color) {
    return svg(`<path d="m0 2.5 l2 -1.5 l1 0 l2 1.5 l1 0" stroke="${color}" fill="none" stroke-width=".7"/>`, `width="6" height="3"`);
}
const baseTheme = /*@__PURE__*/dist/* EditorView */.Lz.baseTheme({
    ".cm-diagnostic": {
        padding: "3px 6px 3px 8px",
        marginLeft: "-1px",
        display: "block",
        whiteSpace: "pre-wrap"
    },
    ".cm-diagnostic-error": { borderLeft: "5px solid #d11" },
    ".cm-diagnostic-warning": { borderLeft: "5px solid orange" },
    ".cm-diagnostic-info": { borderLeft: "5px solid #999" },
    ".cm-diagnostic-hint": { borderLeft: "5px solid #66d" },
    ".cm-diagnosticAction": {
        font: "inherit",
        border: "none",
        padding: "2px 4px",
        backgroundColor: "#444",
        color: "white",
        borderRadius: "3px",
        marginLeft: "8px",
        cursor: "pointer"
    },
    ".cm-diagnosticSource": {
        fontSize: "70%",
        opacity: .7
    },
    ".cm-lintRange": {
        backgroundPosition: "left bottom",
        backgroundRepeat: "repeat-x",
        paddingBottom: "0.7px",
    },
    ".cm-lintRange-error": { backgroundImage: /*@__PURE__*/underline("#d11") },
    ".cm-lintRange-warning": { backgroundImage: /*@__PURE__*/underline("orange") },
    ".cm-lintRange-info": { backgroundImage: /*@__PURE__*/underline("#999") },
    ".cm-lintRange-hint": { backgroundImage: /*@__PURE__*/underline("#66d") },
    ".cm-lintRange-active": { backgroundColor: "#ffdd9980" },
    ".cm-tooltip-lint": {
        padding: 0,
        margin: 0
    },
    ".cm-lintPoint": {
        position: "relative",
        "&:after": {
            content: '""',
            position: "absolute",
            bottom: 0,
            left: "-2px",
            borderLeft: "3px solid transparent",
            borderRight: "3px solid transparent",
            borderBottom: "4px solid #d11"
        }
    },
    ".cm-lintPoint-warning": {
        "&:after": { borderBottomColor: "orange" }
    },
    ".cm-lintPoint-info": {
        "&:after": { borderBottomColor: "#999" }
    },
    ".cm-lintPoint-hint": {
        "&:after": { borderBottomColor: "#66d" }
    },
    ".cm-panel.cm-panel-lint": {
        position: "relative",
        "& ul": {
            maxHeight: "100px",
            overflowY: "auto",
            "& [aria-selected]": {
                backgroundColor: "#ddd",
                "& u": { textDecoration: "underline" }
            },
            "&:focus [aria-selected]": {
                background_fallback: "#bdf",
                backgroundColor: "Highlight",
                color_fallback: "white",
                color: "HighlightText"
            },
            "& u": { textDecoration: "none" },
            padding: 0,
            margin: 0
        },
        "& [name=close]": {
            position: "absolute",
            top: "0",
            right: "2px",
            background: "inherit",
            border: "none",
            font: "inherit",
            padding: 0,
            margin: 0
        }
    },
    "&dark .cm-lintRange-active": { backgroundColor: "#86714a80" },
    "&dark .cm-panel.cm-panel-lint ul": {
        "& [aria-selected]": {
            backgroundColor: "#2e343e",
        },
    }
});
function severityWeight(sev) {
    return sev == "error" ? 4 : sev == "warning" ? 3 : sev == "info" ? 2 : 1;
}
function maxSeverity(diagnostics) {
    let sev = "hint", weight = 1;
    for (let d of diagnostics) {
        let w = severityWeight(d.severity);
        if (w > weight) {
            weight = w;
            sev = d.severity;
        }
    }
    return sev;
}
class LintGutterMarker extends dist/* GutterMarker */.wJ {
    constructor(diagnostics) {
        super();
        this.diagnostics = diagnostics;
        this.severity = maxSeverity(diagnostics);
    }
    toDOM(view) {
        let elt = document.createElement("div");
        elt.className = "cm-lint-marker cm-lint-marker-" + this.severity;
        let diagnostics = this.diagnostics;
        let diagnosticsFilter = view.state.facet(lintGutterConfig).tooltipFilter;
        if (diagnosticsFilter)
            diagnostics = diagnosticsFilter(diagnostics, view.state);
        if (diagnostics.length)
            elt.onmouseover = () => gutterMarkerMouseOver(view, elt, diagnostics);
        return elt;
    }
}
function trackHoverOn(view, marker) {
    let mousemove = (event) => {
        let rect = marker.getBoundingClientRect();
        if (event.clientX > rect.left - 10 /* Hover.Margin */ && event.clientX < rect.right + 10 /* Hover.Margin */ &&
            event.clientY > rect.top - 10 /* Hover.Margin */ && event.clientY < rect.bottom + 10 /* Hover.Margin */)
            return;
        for (let target = event.target; target; target = target.parentNode) {
            if (target.nodeType == 1 && target.classList.contains("cm-tooltip-lint"))
                return;
        }
        window.removeEventListener("mousemove", mousemove);
        if (view.state.field(lintGutterTooltip))
            view.dispatch({ effects: setLintGutterTooltip.of(null) });
    };
    window.addEventListener("mousemove", mousemove);
}
function gutterMarkerMouseOver(view, marker, diagnostics) {
    function hovered() {
        let line = view.elementAtHeight(marker.getBoundingClientRect().top + 5 - view.documentTop);
        const linePos = view.coordsAtPos(line.from);
        if (linePos) {
            view.dispatch({ effects: setLintGutterTooltip.of({
                    pos: line.from,
                    above: false,
                    clip: false,
                    create() {
                        return {
                            dom: diagnosticsTooltip(view, diagnostics),
                            getCoords: () => marker.getBoundingClientRect()
                        };
                    }
                }) });
        }
        marker.onmouseout = marker.onmousemove = null;
        trackHoverOn(view, marker);
    }
    let { hoverTime } = view.state.facet(lintGutterConfig);
    let hoverTimeout = setTimeout(hovered, hoverTime);
    marker.onmouseout = () => {
        clearTimeout(hoverTimeout);
        marker.onmouseout = marker.onmousemove = null;
    };
    marker.onmousemove = () => {
        clearTimeout(hoverTimeout);
        hoverTimeout = setTimeout(hovered, hoverTime);
    };
}
function markersForDiagnostics(doc, diagnostics) {
    let byLine = Object.create(null);
    for (let diagnostic of diagnostics) {
        let line = doc.lineAt(diagnostic.from);
        (byLine[line.from] || (byLine[line.from] = [])).push(diagnostic);
    }
    let markers = [];
    for (let line in byLine) {
        markers.push(new LintGutterMarker(byLine[line]).range(+line));
    }
    return state_dist/* RangeSet */.om.of(markers, true);
}
const lintGutterExtension = /*@__PURE__*/(0,dist/* gutter */.cU)({
    class: "cm-gutter-lint",
    markers: view => view.state.field(lintGutterMarkers),
    widgetMarker: (view, widget, block) => {
        let diagnostics = [];
        view.state.field(lintGutterMarkers).between(block.from, block.to, (from, to, value) => {
            if (from > block.from && from < block.to)
                diagnostics.push(...value.diagnostics);
        });
        return diagnostics.length ? new LintGutterMarker(diagnostics) : null;
    }
});
const lintGutterMarkers = /*@__PURE__*/state_dist/* StateField */.sU.define({
    create() {
        return state_dist/* RangeSet */.om.empty;
    },
    update(markers, tr) {
        markers = markers.map(tr.changes);
        let diagnosticFilter = tr.state.facet(lintGutterConfig).markerFilter;
        for (let effect of tr.effects) {
            if (effect.is(setDiagnosticsEffect)) {
                let diagnostics = effect.value;
                if (diagnosticFilter)
                    diagnostics = diagnosticFilter(diagnostics || [], tr.state);
                markers = markersForDiagnostics(tr.state.doc, diagnostics.slice(0));
            }
        }
        return markers;
    }
});
const setLintGutterTooltip = /*@__PURE__*/state_dist/* StateEffect */.Pe.define();
const lintGutterTooltip = /*@__PURE__*/state_dist/* StateField */.sU.define({
    create() { return null; },
    update(tooltip, tr) {
        if (tooltip && tr.docChanged)
            tooltip = hideTooltip(tr, tooltip) ? null : { ...tooltip, pos: tr.changes.mapPos(tooltip.pos) };
        return tr.effects.reduce((t, e) => e.is(setLintGutterTooltip) ? e.value : t, tooltip);
    },
    provide: field => dist/* showTooltip */.DK.from(field)
});
const lintGutterTheme = /*@__PURE__*/dist/* EditorView */.Lz.baseTheme({
    ".cm-gutter-lint": {
        width: "1.4em",
        "& .cm-gutterElement": {
            padding: ".2em"
        }
    },
    ".cm-lint-marker": {
        width: "1em",
        height: "1em"
    },
    ".cm-lint-marker-info": {
        content: /*@__PURE__*/svg(`<path fill="#aaf" stroke="#77e" stroke-width="6" stroke-linejoin="round" d="M5 5L35 5L35 35L5 35Z"/>`)
    },
    ".cm-lint-marker-warning": {
        content: /*@__PURE__*/svg(`<path fill="#fe8" stroke="#fd7" stroke-width="6" stroke-linejoin="round" d="M20 6L37 35L3 35Z"/>`),
    },
    ".cm-lint-marker-error": {
        content: /*@__PURE__*/svg(`<circle cx="20" cy="20" r="15" fill="#f87" stroke="#f43" stroke-width="6"/>`)
    },
});
const lintExtensions = [
    lintState,
    /*@__PURE__*/dist/* EditorView */.Lz.decorations.compute([lintState], state => {
        let { selected, panel } = state.field(lintState);
        return !selected || !panel || selected.from == selected.to ? dist/* Decoration */.NZ.none : dist/* Decoration */.NZ.set([
            activeMark.range(selected.from, selected.to)
        ]);
    }),
    /*@__PURE__*/(0,dist/* hoverTooltip */.Ux)(lintTooltip, { hideOn: hideTooltip }),
    baseTheme
];
const lintGutterConfig = /*@__PURE__*/state_dist/* Facet */.sj.define({
    combine(configs) {
        return (0,state_dist/* combineConfig */.QR)(configs, {
            hoverTime: 300 /* Hover.Time */,
            markerFilter: null,
            tooltipFilter: null
        });
    }
});
/**
Returns an extension that installs a gutter showing markers for
each line that has diagnostics, which can be hovered over to see
the diagnostics.
*/
function lintGutter(config = {}) {
    return [lintGutterConfig.of(config), lintGutterMarkers, lintGutterExtension, lintGutterTheme, lintGutterTooltip];
}
/**
Iterate over the marked diagnostics for the given editor state,
calling `f` for each of them. Note that, if the document changed
since the diagnostics were created, the `Diagnostic` object will
hold the original outdated position, whereas the `to` and `from`
arguments hold the diagnostic's current position.
*/
function forEachDiagnostic(state, f) {
    let lState = state.field(lintState, false);
    if (lState && lState.diagnostics.size) {
        let pending = [], pendingStart = [], lastEnd = -1;
        for (let iter = RangeSet.iter([lState.diagnostics]);; iter.next()) {
            for (let i = 0; i < pending.length; i++)
                if (!iter.value || iter.value.spec.diagnostics.indexOf(pending[i]) < 0) {
                    f(pending[i], pendingStart[i], lastEnd);
                    pending.splice(i, 1);
                    pendingStart.splice(i--, 1);
                }
            if (!iter.value)
                break;
            for (let d of iter.value.spec.diagnostics)
                if (pending.indexOf(d) < 0) {
                    pending.push(d);
                    pendingStart.push(iter.from);
                }
            lastEnd = iter.to;
        }
    }
}



;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/codemirror@6.0.2/node_modules/codemirror/dist/index.js









// (The superfluous function calls around the list of extensions work
// around current limitations in tree-shaking software.)
/**
This is an extension value that just pulls together a number of
extensions that you might want in a basic editor. It is meant as a
convenient helper to quickly set up CodeMirror without installing
and importing a lot of separate packages.

Specifically, it includes...

 - [the default command bindings](https://codemirror.net/6/docs/ref/#commands.defaultKeymap)
 - [line numbers](https://codemirror.net/6/docs/ref/#view.lineNumbers)
 - [special character highlighting](https://codemirror.net/6/docs/ref/#view.highlightSpecialChars)
 - [the undo history](https://codemirror.net/6/docs/ref/#commands.history)
 - [a fold gutter](https://codemirror.net/6/docs/ref/#language.foldGutter)
 - [custom selection drawing](https://codemirror.net/6/docs/ref/#view.drawSelection)
 - [drop cursor](https://codemirror.net/6/docs/ref/#view.dropCursor)
 - [multiple selections](https://codemirror.net/6/docs/ref/#state.EditorState^allowMultipleSelections)
 - [reindentation on input](https://codemirror.net/6/docs/ref/#language.indentOnInput)
 - [the default highlight style](https://codemirror.net/6/docs/ref/#language.defaultHighlightStyle) (as fallback)
 - [bracket matching](https://codemirror.net/6/docs/ref/#language.bracketMatching)
 - [bracket closing](https://codemirror.net/6/docs/ref/#autocomplete.closeBrackets)
 - [autocompletion](https://codemirror.net/6/docs/ref/#autocomplete.autocompletion)
 - [rectangular selection](https://codemirror.net/6/docs/ref/#view.rectangularSelection) and [crosshair cursor](https://codemirror.net/6/docs/ref/#view.crosshairCursor)
 - [active line highlighting](https://codemirror.net/6/docs/ref/#view.highlightActiveLine)
 - [active line gutter highlighting](https://codemirror.net/6/docs/ref/#view.highlightActiveLineGutter)
 - [selection match highlighting](https://codemirror.net/6/docs/ref/#search.highlightSelectionMatches)
 - [search](https://codemirror.net/6/docs/ref/#search.searchKeymap)
 - [linting](https://codemirror.net/6/docs/ref/#lint.lintKeymap)

(You'll probably want to add some language package to your setup
too.)

This extension does not allow customization. The idea is that,
once you decide you want to configure your editor more precisely,
you take this package's source (which is just a bunch of imports
and an array literal), copy it into your own code, and adjust it
as desired.
*/
const basicSetup = /*@__PURE__*/(() => [
    (0,dist/* lineNumbers */.$K)(),
    (0,dist/* highlightActiveLineGutter */.Wu)(),
    (0,dist/* highlightSpecialChars */.N$)(),
    (0,commands_dist/* history */.b6)(),
    (0,language_dist/* foldGutter */.Lv)(),
    (0,dist/* drawSelection */.VH)(),
    (0,dist/* dropCursor */.A)(),
    state_dist/* EditorState */.$t.allowMultipleSelections.of(true),
    (0,language_dist/* indentOnInput */.WD)(),
    (0,language_dist/* syntaxHighlighting */.y9)(language_dist/* defaultHighlightStyle */.Zt, { fallback: true }),
    (0,language_dist/* bracketMatching */.SG)(),
    (0,autocomplete_dist/* closeBrackets */.wm)(),
    (0,autocomplete_dist/* autocompletion */.yU)(),
    (0,dist/* rectangularSelection */.D4)(),
    (0,dist/* crosshairCursor */.HJ)(),
    (0,dist/* highlightActiveLine */.dz)(),
    (0,search_dist/* highlightSelectionMatches */.gN)(),
    dist/* keymap */.w4.of([
        ...autocomplete_dist/* closeBracketsKeymap */.Bc,
        ...commands_dist/* defaultKeymap */.pw,
        ...search_dist/* searchKeymap */.Eo,
        ...commands_dist/* historyKeymap */.cL,
        ...language_dist/* foldKeymap */.f7,
        ...autocomplete_dist/* completionKeymap */.OO,
        ...lintKeymap
    ])
])();
/**
A minimal set of extensions to create a functional editor. Only
includes [the default keymap](https://codemirror.net/6/docs/ref/#commands.defaultKeymap), [undo
history](https://codemirror.net/6/docs/ref/#commands.history), [special character
highlighting](https://codemirror.net/6/docs/ref/#view.highlightSpecialChars), [custom selection
drawing](https://codemirror.net/6/docs/ref/#view.drawSelection), and [default highlight
style](https://codemirror.net/6/docs/ref/#language.defaultHighlightStyle).
*/
const minimalSetup = /*@__PURE__*/(() => [
    (0,dist/* highlightSpecialChars */.N$)(),
    (0,commands_dist/* history */.b6)(),
    (0,dist/* drawSelection */.VH)(),
    (0,language_dist/* syntaxHighlighting */.y9)(language_dist/* defaultHighlightStyle */.Zt, { fallback: true }),
    dist/* keymap */.w4.of([
        ...commands_dist/* defaultKeymap */.pw,
        ...commands_dist/* historyKeymap */.cL,
    ])
])();




/***/ }),

/***/ 38605:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ar: () => (/* binding */ ifNotIn),
/* harmony export */   Bc: () => (/* binding */ closeBracketsKeymap),
/* harmony export */   Gw: () => (/* binding */ snippetCompletion),
/* harmony export */   OO: () => (/* binding */ completionKeymap),
/* harmony export */   _5: () => (/* binding */ CompletionContext),
/* harmony export */   et: () => (/* binding */ completeFromList),
/* harmony export */   wm: () => (/* binding */ closeBrackets),
/* harmony export */   yU: () => (/* binding */ autocompletion)
/* harmony export */ });
/* unused harmony exports acceptCompletion, clearSnippet, closeCompletion, completeAnyWord, completionStatus, currentCompletions, deleteBracketPair, hasNextSnippetField, hasPrevSnippetField, ifIn, insertBracket, insertCompletionText, moveCompletionSelection, nextSnippetField, pickedCompletion, prevSnippetField, selectedCompletion, selectedCompletionIndex, setSelectedCompletion, snippet, snippetKeymap, startCompletion */
/* harmony import */ var _codemirror_state__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(64614);
/* harmony import */ var _codemirror_view__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(87748);
/* harmony import */ var _codemirror_language__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(35088);




/**
An instance of this is passed to completion source functions.
*/
class CompletionContext {
    /**
    Create a new completion context. (Mostly useful for testing
    completion sources—in the editor, the extension will create
    these for you.)
    */
    constructor(
    /**
    The editor state that the completion happens in.
    */
    state, 
    /**
    The position at which the completion is happening.
    */
    pos, 
    /**
    Indicates whether completion was activated explicitly, or
    implicitly by typing. The usual way to respond to this is to
    only return completions when either there is part of a
    completable entity before the cursor, or `explicit` is true.
    */
    explicit, 
    /**
    The editor view. May be undefined if the context was created
    in a situation where there is no such view available, such as
    in synchronous updates via
    [`CompletionResult.update`](https://codemirror.net/6/docs/ref/#autocomplete.CompletionResult.update)
    or when called by test code.
    */
    view) {
        this.state = state;
        this.pos = pos;
        this.explicit = explicit;
        this.view = view;
        /**
        @internal
        */
        this.abortListeners = [];
        /**
        @internal
        */
        this.abortOnDocChange = false;
    }
    /**
    Get the extent, content, and (if there is a token) type of the
    token before `this.pos`.
    */
    tokenBefore(types) {
        let token = (0,_codemirror_language__WEBPACK_IMPORTED_MODULE_0__/* .syntaxTree */ .mv)(this.state).resolveInner(this.pos, -1);
        while (token && types.indexOf(token.name) < 0)
            token = token.parent;
        return token ? { from: token.from, to: this.pos,
            text: this.state.sliceDoc(token.from, this.pos),
            type: token.type } : null;
    }
    /**
    Get the match of the given expression directly before the
    cursor.
    */
    matchBefore(expr) {
        let line = this.state.doc.lineAt(this.pos);
        let start = Math.max(line.from, this.pos - 250);
        let str = line.text.slice(start - line.from, this.pos - line.from);
        let found = str.search(ensureAnchor(expr, false));
        return found < 0 ? null : { from: start + found, to: this.pos, text: str.slice(found) };
    }
    /**
    Yields true when the query has been aborted. Can be useful in
    asynchronous queries to avoid doing work that will be ignored.
    */
    get aborted() { return this.abortListeners == null; }
    /**
    Allows you to register abort handlers, which will be called when
    the query is
    [aborted](https://codemirror.net/6/docs/ref/#autocomplete.CompletionContext.aborted).
    
    By default, running queries will not be aborted for regular
    typing or backspacing, on the assumption that they are likely to
    return a result with a
    [`validFor`](https://codemirror.net/6/docs/ref/#autocomplete.CompletionResult.validFor) field that
    allows the result to be used after all. Passing `onDocChange:
    true` will cause this query to be aborted for any document
    change.
    */
    addEventListener(type, listener, options) {
        if (type == "abort" && this.abortListeners) {
            this.abortListeners.push(listener);
            if (options && options.onDocChange)
                this.abortOnDocChange = true;
        }
    }
}
function toSet(chars) {
    let flat = Object.keys(chars).join("");
    let words = /\w/.test(flat);
    if (words)
        flat = flat.replace(/\w/g, "");
    return `[${words ? "\\w" : ""}${flat.replace(/[^\w\s]/g, "\\$&")}]`;
}
function prefixMatch(options) {
    let first = Object.create(null), rest = Object.create(null);
    for (let { label } of options) {
        first[label[0]] = true;
        for (let i = 1; i < label.length; i++)
            rest[label[i]] = true;
    }
    let source = toSet(first) + toSet(rest) + "*$";
    return [new RegExp("^" + source), new RegExp(source)];
}
/**
Given a a fixed array of options, return an autocompleter that
completes them.
*/
function completeFromList(list) {
    let options = list.map(o => typeof o == "string" ? { label: o } : o);
    let [validFor, match] = options.every(o => /^\w+$/.test(o.label)) ? [/\w*$/, /\w+$/] : prefixMatch(options);
    return (context) => {
        let token = context.matchBefore(match);
        return token || context.explicit ? { from: token ? token.from : context.pos, options, validFor } : null;
    };
}
/**
Wrap the given completion source so that it will only fire when the
cursor is in a syntax node with one of the given names.
*/
function ifIn(nodes, source) {
    return (context) => {
        for (let pos = syntaxTree(context.state).resolveInner(context.pos, -1); pos; pos = pos.parent) {
            if (nodes.indexOf(pos.name) > -1)
                return source(context);
            if (pos.type.isTop)
                break;
        }
        return null;
    };
}
/**
Wrap the given completion source so that it will not fire when the
cursor is in a syntax node with one of the given names.
*/
function ifNotIn(nodes, source) {
    return (context) => {
        for (let pos = (0,_codemirror_language__WEBPACK_IMPORTED_MODULE_0__/* .syntaxTree */ .mv)(context.state).resolveInner(context.pos, -1); pos; pos = pos.parent) {
            if (nodes.indexOf(pos.name) > -1)
                return null;
            if (pos.type.isTop)
                break;
        }
        return source(context);
    };
}
class Option {
    constructor(completion, source, match, score) {
        this.completion = completion;
        this.source = source;
        this.match = match;
        this.score = score;
    }
}
function cur(state) { return state.selection.main.from; }
// Make sure the given regexp has a $ at its end and, if `start` is
// true, a ^ at its start.
function ensureAnchor(expr, start) {
    var _a;
    let { source } = expr;
    let addStart = start && source[0] != "^", addEnd = source[source.length - 1] != "$";
    if (!addStart && !addEnd)
        return expr;
    return new RegExp(`${addStart ? "^" : ""}(?:${source})${addEnd ? "$" : ""}`, (_a = expr.flags) !== null && _a !== void 0 ? _a : (expr.ignoreCase ? "i" : ""));
}
/**
This annotation is added to transactions that are produced by
picking a completion.
*/
const pickedCompletion = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .Annotation */ .YH.define();
/**
Helper function that returns a transaction spec which inserts a
completion's text in the main selection range, and any other
selection range that has the same text in front of it.
*/
function insertCompletionText(state, text, from, to) {
    let { main } = state.selection, fromOff = from - main.from, toOff = to - main.from;
    return {
        ...state.changeByRange(range => {
            if (range != main && from != to &&
                state.sliceDoc(range.from + fromOff, range.from + toOff) != state.sliceDoc(from, to))
                return { range };
            let lines = state.toText(text);
            return {
                changes: { from: range.from + fromOff, to: to == main.from ? range.to : range.from + toOff, insert: lines },
                range: _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .EditorSelection */ .OF.cursor(range.from + fromOff + lines.length)
            };
        }),
        scrollIntoView: true,
        userEvent: "input.complete"
    };
}
const SourceCache = /*@__PURE__*/new WeakMap();
function asSource(source) {
    if (!Array.isArray(source))
        return source;
    let known = SourceCache.get(source);
    if (!known)
        SourceCache.set(source, known = completeFromList(source));
    return known;
}
const startCompletionEffect = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .StateEffect */ .Pe.define();
const closeCompletionEffect = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .StateEffect */ .Pe.define();

// A pattern matcher for fuzzy completion matching. Create an instance
// once for a pattern, and then use that to match any number of
// completions.
class FuzzyMatcher {
    constructor(pattern) {
        this.pattern = pattern;
        this.chars = [];
        this.folded = [];
        // Buffers reused by calls to `match` to track matched character
        // positions.
        this.any = [];
        this.precise = [];
        this.byWord = [];
        this.score = 0;
        this.matched = [];
        for (let p = 0; p < pattern.length;) {
            let char = (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointAt */ .vS)(pattern, p), size = (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointSize */ .Fh)(char);
            this.chars.push(char);
            let part = pattern.slice(p, p + size), upper = part.toUpperCase();
            this.folded.push((0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointAt */ .vS)(upper == part ? part.toLowerCase() : upper, 0));
            p += size;
        }
        this.astral = pattern.length != this.chars.length;
    }
    ret(score, matched) {
        this.score = score;
        this.matched = matched;
        return this;
    }
    // Matches a given word (completion) against the pattern (input).
    // Will return a boolean indicating whether there was a match and,
    // on success, set `this.score` to the score, `this.matched` to an
    // array of `from, to` pairs indicating the matched parts of `word`.
    //
    // The score is a number that is more negative the worse the match
    // is. See `Penalty` above.
    match(word) {
        if (this.pattern.length == 0)
            return this.ret(-100 /* Penalty.NotFull */, []);
        if (word.length < this.pattern.length)
            return null;
        let { chars, folded, any, precise, byWord } = this;
        // For single-character queries, only match when they occur right
        // at the start
        if (chars.length == 1) {
            let first = (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointAt */ .vS)(word, 0), firstSize = (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointSize */ .Fh)(first);
            let score = firstSize == word.length ? 0 : -100 /* Penalty.NotFull */;
            if (first == chars[0]) ;
            else if (first == folded[0])
                score += -200 /* Penalty.CaseFold */;
            else
                return null;
            return this.ret(score, [0, firstSize]);
        }
        let direct = word.indexOf(this.pattern);
        if (direct == 0)
            return this.ret(word.length == this.pattern.length ? 0 : -100 /* Penalty.NotFull */, [0, this.pattern.length]);
        let len = chars.length, anyTo = 0;
        if (direct < 0) {
            for (let i = 0, e = Math.min(word.length, 200); i < e && anyTo < len;) {
                let next = (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointAt */ .vS)(word, i);
                if (next == chars[anyTo] || next == folded[anyTo])
                    any[anyTo++] = i;
                i += (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointSize */ .Fh)(next);
            }
            // No match, exit immediately
            if (anyTo < len)
                return null;
        }
        // This tracks the extent of the precise (non-folded, not
        // necessarily adjacent) match
        let preciseTo = 0;
        // Tracks whether there is a match that hits only characters that
        // appear to be starting words. `byWordFolded` is set to true when
        // a case folded character is encountered in such a match
        let byWordTo = 0, byWordFolded = false;
        // If we've found a partial adjacent match, these track its state
        let adjacentTo = 0, adjacentStart = -1, adjacentEnd = -1;
        let hasLower = /[a-z]/.test(word), wordAdjacent = true;
        // Go over the option's text, scanning for the various kinds of matches
        for (let i = 0, e = Math.min(word.length, 200), prevType = 0 /* Tp.NonWord */; i < e && byWordTo < len;) {
            let next = (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointAt */ .vS)(word, i);
            if (direct < 0) {
                if (preciseTo < len && next == chars[preciseTo])
                    precise[preciseTo++] = i;
                if (adjacentTo < len) {
                    if (next == chars[adjacentTo] || next == folded[adjacentTo]) {
                        if (adjacentTo == 0)
                            adjacentStart = i;
                        adjacentEnd = i + 1;
                        adjacentTo++;
                    }
                    else {
                        adjacentTo = 0;
                    }
                }
            }
            let ch, type = next < 0xff
                ? (next >= 48 && next <= 57 || next >= 97 && next <= 122 ? 2 /* Tp.Lower */ : next >= 65 && next <= 90 ? 1 /* Tp.Upper */ : 0 /* Tp.NonWord */)
                : ((ch = (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .fromCodePoint */ .MK)(next)) != ch.toLowerCase() ? 1 /* Tp.Upper */ : ch != ch.toUpperCase() ? 2 /* Tp.Lower */ : 0 /* Tp.NonWord */);
            if (!i || type == 1 /* Tp.Upper */ && hasLower || prevType == 0 /* Tp.NonWord */ && type != 0 /* Tp.NonWord */) {
                if (chars[byWordTo] == next || (folded[byWordTo] == next && (byWordFolded = true)))
                    byWord[byWordTo++] = i;
                else if (byWord.length)
                    wordAdjacent = false;
            }
            prevType = type;
            i += (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointSize */ .Fh)(next);
        }
        if (byWordTo == len && byWord[0] == 0 && wordAdjacent)
            return this.result(-100 /* Penalty.ByWord */ + (byWordFolded ? -200 /* Penalty.CaseFold */ : 0), byWord, word);
        if (adjacentTo == len && adjacentStart == 0)
            return this.ret(-200 /* Penalty.CaseFold */ - word.length + (adjacentEnd == word.length ? 0 : -100 /* Penalty.NotFull */), [0, adjacentEnd]);
        if (direct > -1)
            return this.ret(-700 /* Penalty.NotStart */ - word.length, [direct, direct + this.pattern.length]);
        if (adjacentTo == len)
            return this.ret(-200 /* Penalty.CaseFold */ + -700 /* Penalty.NotStart */ - word.length, [adjacentStart, adjacentEnd]);
        if (byWordTo == len)
            return this.result(-100 /* Penalty.ByWord */ + (byWordFolded ? -200 /* Penalty.CaseFold */ : 0) + -700 /* Penalty.NotStart */ +
                (wordAdjacent ? 0 : -1100 /* Penalty.Gap */), byWord, word);
        return chars.length == 2 ? null
            : this.result((any[0] ? -700 /* Penalty.NotStart */ : 0) + -200 /* Penalty.CaseFold */ + -1100 /* Penalty.Gap */, any, word);
    }
    result(score, positions, word) {
        let result = [], i = 0;
        for (let pos of positions) {
            let to = pos + (this.astral ? (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointSize */ .Fh)((0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointAt */ .vS)(word, pos)) : 1);
            if (i && result[i - 1] == pos)
                result[i - 1] = to;
            else {
                result[i++] = pos;
                result[i++] = to;
            }
        }
        return this.ret(score - word.length, result);
    }
}
class StrictMatcher {
    constructor(pattern) {
        this.pattern = pattern;
        this.matched = [];
        this.score = 0;
        this.folded = pattern.toLowerCase();
    }
    match(word) {
        if (word.length < this.pattern.length)
            return null;
        let start = word.slice(0, this.pattern.length);
        let match = start == this.pattern ? 0 : start.toLowerCase() == this.folded ? -200 /* Penalty.CaseFold */ : null;
        if (match == null)
            return null;
        this.matched = [0, start.length];
        this.score = match + (word.length == this.pattern.length ? 0 : -100 /* Penalty.NotFull */);
        return this;
    }
}

const completionConfig = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .Facet */ .sj.define({
    combine(configs) {
        return (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .combineConfig */ .QR)(configs, {
            activateOnTyping: true,
            activateOnCompletion: () => false,
            activateOnTypingDelay: 100,
            selectOnOpen: true,
            override: null,
            closeOnBlur: true,
            maxRenderedOptions: 100,
            defaultKeymap: true,
            tooltipClass: () => "",
            optionClass: () => "",
            aboveCursor: false,
            icons: true,
            addToOptions: [],
            positionInfo: defaultPositionInfo,
            filterStrict: false,
            compareCompletions: (a, b) => (a.sortText || a.label).localeCompare(b.sortText || b.label),
            interactionDelay: 75,
            updateSyncTime: 100
        }, {
            defaultKeymap: (a, b) => a && b,
            closeOnBlur: (a, b) => a && b,
            icons: (a, b) => a && b,
            tooltipClass: (a, b) => c => joinClass(a(c), b(c)),
            optionClass: (a, b) => c => joinClass(a(c), b(c)),
            addToOptions: (a, b) => a.concat(b),
            filterStrict: (a, b) => a || b,
        });
    }
});
function joinClass(a, b) {
    return a ? b ? a + " " + b : a : b;
}
function defaultPositionInfo(view, list, option, info, space, tooltip) {
    let rtl = view.textDirection == _codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .Direction */ .OP.RTL, left = rtl, narrow = false;
    let side = "top", offset, maxWidth;
    let spaceLeft = list.left - space.left, spaceRight = space.right - list.right;
    let infoWidth = info.right - info.left, infoHeight = info.bottom - info.top;
    if (left && spaceLeft < Math.min(infoWidth, spaceRight))
        left = false;
    else if (!left && spaceRight < Math.min(infoWidth, spaceLeft))
        left = true;
    if (infoWidth <= (left ? spaceLeft : spaceRight)) {
        offset = Math.max(space.top, Math.min(option.top, space.bottom - infoHeight)) - list.top;
        maxWidth = Math.min(400 /* Info.Width */, left ? spaceLeft : spaceRight);
    }
    else {
        narrow = true;
        maxWidth = Math.min(400 /* Info.Width */, (rtl ? list.right : space.right - list.left) - 30 /* Info.Margin */);
        let spaceBelow = space.bottom - list.bottom;
        if (spaceBelow >= infoHeight || spaceBelow > list.top) { // Below the completion
            offset = option.bottom - list.top;
        }
        else { // Above it
            side = "bottom";
            offset = list.bottom - option.top;
        }
    }
    let scaleY = (list.bottom - list.top) / tooltip.offsetHeight;
    let scaleX = (list.right - list.left) / tooltip.offsetWidth;
    return {
        style: `${side}: ${offset / scaleY}px; max-width: ${maxWidth / scaleX}px`,
        class: "cm-completionInfo-" + (narrow ? (rtl ? "left-narrow" : "right-narrow") : left ? "left" : "right")
    };
}

const setSelectedEffect = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .StateEffect */ .Pe.define();
function optionContent(config) {
    let content = config.addToOptions.slice();
    if (config.icons)
        content.push({
            render(completion) {
                let icon = document.createElement("div");
                icon.classList.add("cm-completionIcon");
                if (completion.type)
                    icon.classList.add(...completion.type.split(/\s+/g).map(cls => "cm-completionIcon-" + cls));
                icon.setAttribute("aria-hidden", "true");
                return icon;
            },
            position: 20
        });
    content.push({
        render(completion, _s, _v, match) {
            let labelElt = document.createElement("span");
            labelElt.className = "cm-completionLabel";
            let label = completion.displayLabel || completion.label, off = 0;
            for (let j = 0; j < match.length;) {
                let from = match[j++], to = match[j++];
                if (from > off)
                    labelElt.appendChild(document.createTextNode(label.slice(off, from)));
                let span = labelElt.appendChild(document.createElement("span"));
                span.appendChild(document.createTextNode(label.slice(from, to)));
                span.className = "cm-completionMatchedText";
                off = to;
            }
            if (off < label.length)
                labelElt.appendChild(document.createTextNode(label.slice(off)));
            return labelElt;
        },
        position: 50
    }, {
        render(completion) {
            if (!completion.detail)
                return null;
            let detailElt = document.createElement("span");
            detailElt.className = "cm-completionDetail";
            detailElt.textContent = completion.detail;
            return detailElt;
        },
        position: 80
    });
    return content.sort((a, b) => a.position - b.position).map(a => a.render);
}
function rangeAroundSelected(total, selected, max) {
    if (total <= max)
        return { from: 0, to: total };
    if (selected < 0)
        selected = 0;
    if (selected <= (total >> 1)) {
        let off = Math.floor(selected / max);
        return { from: off * max, to: (off + 1) * max };
    }
    let off = Math.floor((total - selected) / max);
    return { from: total - (off + 1) * max, to: total - off * max };
}
class CompletionTooltip {
    constructor(view, stateField, applyCompletion) {
        this.view = view;
        this.stateField = stateField;
        this.applyCompletion = applyCompletion;
        this.info = null;
        this.infoDestroy = null;
        this.placeInfoReq = {
            read: () => this.measureInfo(),
            write: (pos) => this.placeInfo(pos),
            key: this
        };
        this.space = null;
        this.currentClass = "";
        let cState = view.state.field(stateField);
        let { options, selected } = cState.open;
        let config = view.state.facet(completionConfig);
        this.optionContent = optionContent(config);
        this.optionClass = config.optionClass;
        this.tooltipClass = config.tooltipClass;
        this.range = rangeAroundSelected(options.length, selected, config.maxRenderedOptions);
        this.dom = document.createElement("div");
        this.dom.className = "cm-tooltip-autocomplete";
        this.updateTooltipClass(view.state);
        this.dom.addEventListener("mousedown", (e) => {
            let { options } = view.state.field(stateField).open;
            for (let dom = e.target, match; dom && dom != this.dom; dom = dom.parentNode) {
                if (dom.nodeName == "LI" && (match = /-(\d+)$/.exec(dom.id)) && +match[1] < options.length) {
                    this.applyCompletion(view, options[+match[1]]);
                    e.preventDefault();
                    return;
                }
            }
            if (e.target == this.list) {
                let move = this.list.classList.contains("cm-completionListIncompleteTop") &&
                    e.clientY < this.list.firstChild.getBoundingClientRect().top ? this.range.from - 1 :
                    this.list.classList.contains("cm-completionListIncompleteBottom") &&
                        e.clientY > this.list.lastChild.getBoundingClientRect().bottom ? this.range.to : null;
                if (move != null) {
                    view.dispatch({ effects: setSelectedEffect.of(move) });
                    e.preventDefault();
                }
            }
        });
        this.dom.addEventListener("focusout", (e) => {
            let state = view.state.field(this.stateField, false);
            if (state && state.tooltip && view.state.facet(completionConfig).closeOnBlur &&
                e.relatedTarget != view.contentDOM)
                view.dispatch({ effects: closeCompletionEffect.of(null) });
        });
        this.showOptions(options, cState.id);
    }
    mount() { this.updateSel(); }
    showOptions(options, id) {
        if (this.list)
            this.list.remove();
        this.list = this.dom.appendChild(this.createListBox(options, id, this.range));
        this.list.addEventListener("scroll", () => {
            if (this.info)
                this.view.requestMeasure(this.placeInfoReq);
        });
    }
    update(update) {
        var _a;
        let cState = update.state.field(this.stateField);
        let prevState = update.startState.field(this.stateField);
        this.updateTooltipClass(update.state);
        if (cState != prevState) {
            let { options, selected, disabled } = cState.open;
            if (!prevState.open || prevState.open.options != options) {
                this.range = rangeAroundSelected(options.length, selected, update.state.facet(completionConfig).maxRenderedOptions);
                this.showOptions(options, cState.id);
            }
            this.updateSel();
            if (disabled != ((_a = prevState.open) === null || _a === void 0 ? void 0 : _a.disabled))
                this.dom.classList.toggle("cm-tooltip-autocomplete-disabled", !!disabled);
        }
    }
    updateTooltipClass(state) {
        let cls = this.tooltipClass(state);
        if (cls != this.currentClass) {
            for (let c of this.currentClass.split(" "))
                if (c)
                    this.dom.classList.remove(c);
            for (let c of cls.split(" "))
                if (c)
                    this.dom.classList.add(c);
            this.currentClass = cls;
        }
    }
    positioned(space) {
        this.space = space;
        if (this.info)
            this.view.requestMeasure(this.placeInfoReq);
    }
    updateSel() {
        let cState = this.view.state.field(this.stateField), open = cState.open;
        if (open.selected > -1 && open.selected < this.range.from || open.selected >= this.range.to) {
            this.range = rangeAroundSelected(open.options.length, open.selected, this.view.state.facet(completionConfig).maxRenderedOptions);
            this.showOptions(open.options, cState.id);
        }
        let newSel = this.updateSelectedOption(open.selected);
        if (newSel) {
            this.destroyInfo();
            let { completion } = open.options[open.selected];
            let { info } = completion;
            if (!info)
                return;
            let infoResult = typeof info === "string" ? document.createTextNode(info) : info(completion);
            if (!infoResult)
                return;
            if ("then" in infoResult) {
                infoResult.then(obj => {
                    if (obj && this.view.state.field(this.stateField, false) == cState)
                        this.addInfoPane(obj, completion);
                }).catch(e => (0,_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .logException */ .c_)(this.view.state, e, "completion info"));
            }
            else {
                this.addInfoPane(infoResult, completion);
                newSel.setAttribute("aria-describedby", this.info.id);
            }
        }
    }
    addInfoPane(content, completion) {
        this.destroyInfo();
        let wrap = this.info = document.createElement("div");
        wrap.className = "cm-tooltip cm-completionInfo";
        wrap.id = "cm-completionInfo-" + Math.floor(Math.random() * 0xffff).toString(16);
        if (content.nodeType != null) {
            wrap.appendChild(content);
            this.infoDestroy = null;
        }
        else {
            let { dom, destroy } = content;
            wrap.appendChild(dom);
            this.infoDestroy = destroy || null;
        }
        this.dom.appendChild(wrap);
        this.view.requestMeasure(this.placeInfoReq);
    }
    updateSelectedOption(selected) {
        let set = null;
        for (let opt = this.list.firstChild, i = this.range.from; opt; opt = opt.nextSibling, i++) {
            if (opt.nodeName != "LI" || !opt.id) {
                i--; // A section header
            }
            else if (i == selected) {
                if (!opt.hasAttribute("aria-selected")) {
                    opt.setAttribute("aria-selected", "true");
                    set = opt;
                }
            }
            else {
                if (opt.hasAttribute("aria-selected")) {
                    opt.removeAttribute("aria-selected");
                    opt.removeAttribute("aria-describedby");
                }
            }
        }
        if (set)
            scrollIntoView(this.list, set);
        return set;
    }
    measureInfo() {
        let sel = this.dom.querySelector("[aria-selected]");
        if (!sel || !this.info)
            return null;
        let listRect = this.dom.getBoundingClientRect();
        let infoRect = this.info.getBoundingClientRect();
        let selRect = sel.getBoundingClientRect();
        let space = this.space;
        if (!space) {
            let docElt = this.dom.ownerDocument.documentElement;
            space = { left: 0, top: 0, right: docElt.clientWidth, bottom: docElt.clientHeight };
        }
        if (selRect.top > Math.min(space.bottom, listRect.bottom) - 10 ||
            selRect.bottom < Math.max(space.top, listRect.top) + 10)
            return null;
        return this.view.state.facet(completionConfig).positionInfo(this.view, listRect, selRect, infoRect, space, this.dom);
    }
    placeInfo(pos) {
        if (this.info) {
            if (pos) {
                if (pos.style)
                    this.info.style.cssText = pos.style;
                this.info.className = "cm-tooltip cm-completionInfo " + (pos.class || "");
            }
            else {
                this.info.style.cssText = "top: -1e6px";
            }
        }
    }
    createListBox(options, id, range) {
        const ul = document.createElement("ul");
        ul.id = id;
        ul.setAttribute("role", "listbox");
        ul.setAttribute("aria-expanded", "true");
        ul.setAttribute("aria-label", this.view.state.phrase("Completions"));
        ul.addEventListener("mousedown", e => {
            // Prevent focus change when clicking the scrollbar
            if (e.target == ul)
                e.preventDefault();
        });
        let curSection = null;
        for (let i = range.from; i < range.to; i++) {
            let { completion, match } = options[i], { section } = completion;
            if (section) {
                let name = typeof section == "string" ? section : section.name;
                if (name != curSection && (i > range.from || range.from == 0)) {
                    curSection = name;
                    if (typeof section != "string" && section.header) {
                        ul.appendChild(section.header(section));
                    }
                    else {
                        let header = ul.appendChild(document.createElement("completion-section"));
                        header.textContent = name;
                    }
                }
            }
            const li = ul.appendChild(document.createElement("li"));
            li.id = id + "-" + i;
            li.setAttribute("role", "option");
            let cls = this.optionClass(completion);
            if (cls)
                li.className = cls;
            for (let source of this.optionContent) {
                let node = source(completion, this.view.state, this.view, match);
                if (node)
                    li.appendChild(node);
            }
        }
        if (range.from)
            ul.classList.add("cm-completionListIncompleteTop");
        if (range.to < options.length)
            ul.classList.add("cm-completionListIncompleteBottom");
        return ul;
    }
    destroyInfo() {
        if (this.info) {
            if (this.infoDestroy)
                this.infoDestroy();
            this.info.remove();
            this.info = null;
        }
    }
    destroy() {
        this.destroyInfo();
    }
}
function completionTooltip(stateField, applyCompletion) {
    return (view) => new CompletionTooltip(view, stateField, applyCompletion);
}
function scrollIntoView(container, element) {
    let parent = container.getBoundingClientRect();
    let self = element.getBoundingClientRect();
    let scaleY = parent.height / container.offsetHeight;
    if (self.top < parent.top)
        container.scrollTop -= (parent.top - self.top) / scaleY;
    else if (self.bottom > parent.bottom)
        container.scrollTop += (self.bottom - parent.bottom) / scaleY;
}

// Used to pick a preferred option when two options with the same
// label occur in the result.
function score(option) {
    return (option.boost || 0) * 100 + (option.apply ? 10 : 0) + (option.info ? 5 : 0) +
        (option.type ? 1 : 0);
}
function sortOptions(active, state) {
    let options = [];
    let sections = null, dynamicSectionScore = null;
    let addOption = (option) => {
        options.push(option);
        let { section } = option.completion;
        if (section) {
            if (!sections)
                sections = [];
            let name = typeof section == "string" ? section : section.name;
            if (!sections.some(s => s.name == name))
                sections.push(typeof section == "string" ? { name } : section);
        }
    };
    let conf = state.facet(completionConfig);
    for (let a of active)
        if (a.hasResult()) {
            let getMatch = a.result.getMatch;
            if (a.result.filter === false) {
                for (let option of a.result.options) {
                    addOption(new Option(option, a.source, getMatch ? getMatch(option) : [], 1e9 - options.length));
                }
            }
            else {
                let pattern = state.sliceDoc(a.from, a.to), match;
                let matcher = conf.filterStrict ? new StrictMatcher(pattern) : new FuzzyMatcher(pattern);
                for (let option of a.result.options)
                    if (match = matcher.match(option.label)) {
                        let matched = !option.displayLabel ? match.matched : getMatch ? getMatch(option, match.matched) : [];
                        let score = match.score + (option.boost || 0);
                        addOption(new Option(option, a.source, matched, score));
                        if (typeof option.section == "object" && option.section.rank === "dynamic") {
                            let { name } = option.section;
                            if (!dynamicSectionScore)
                                dynamicSectionScore = Object.create(null);
                            dynamicSectionScore[name] = Math.max(score, dynamicSectionScore[name] || -1e9);
                        }
                    }
            }
        }
    if (sections) {
        let sectionOrder = Object.create(null), pos = 0;
        let cmp = (a, b) => {
            return (a.rank === "dynamic" && b.rank === "dynamic" ? dynamicSectionScore[b.name] - dynamicSectionScore[a.name] : 0) ||
                (typeof a.rank == "number" ? a.rank : 1e9) - (typeof b.rank == "number" ? b.rank : 1e9) ||
                (a.name < b.name ? -1 : 1);
        };
        for (let s of sections.sort(cmp)) {
            pos -= 1e5;
            sectionOrder[s.name] = pos;
        }
        for (let option of options) {
            let { section } = option.completion;
            if (section)
                option.score += sectionOrder[typeof section == "string" ? section : section.name];
        }
    }
    let result = [], prev = null;
    let compare = conf.compareCompletions;
    for (let opt of options.sort((a, b) => (b.score - a.score) || compare(a.completion, b.completion))) {
        let cur = opt.completion;
        if (!prev || prev.label != cur.label || prev.detail != cur.detail ||
            (prev.type != null && cur.type != null && prev.type != cur.type) ||
            prev.apply != cur.apply || prev.boost != cur.boost)
            result.push(opt);
        else if (score(opt.completion) > score(prev))
            result[result.length - 1] = opt;
        prev = opt.completion;
    }
    return result;
}
class CompletionDialog {
    constructor(options, attrs, tooltip, timestamp, selected, disabled) {
        this.options = options;
        this.attrs = attrs;
        this.tooltip = tooltip;
        this.timestamp = timestamp;
        this.selected = selected;
        this.disabled = disabled;
    }
    setSelected(selected, id) {
        return selected == this.selected || selected >= this.options.length ? this
            : new CompletionDialog(this.options, makeAttrs(id, selected), this.tooltip, this.timestamp, selected, this.disabled);
    }
    static build(active, state, id, prev, conf, didSetActive) {
        if (prev && !didSetActive && active.some(s => s.isPending))
            return prev.setDisabled();
        let options = sortOptions(active, state);
        if (!options.length)
            return prev && active.some(a => a.isPending) ? prev.setDisabled() : null;
        let selected = state.facet(completionConfig).selectOnOpen ? 0 : -1;
        if (prev && prev.selected != selected && prev.selected != -1) {
            let selectedValue = prev.options[prev.selected].completion;
            for (let i = 0; i < options.length; i++)
                if (options[i].completion == selectedValue) {
                    selected = i;
                    break;
                }
        }
        return new CompletionDialog(options, makeAttrs(id, selected), {
            pos: active.reduce((a, b) => b.hasResult() ? Math.min(a, b.from) : a, 1e8),
            create: createTooltip,
            above: conf.aboveCursor,
        }, prev ? prev.timestamp : Date.now(), selected, false);
    }
    map(changes) {
        return new CompletionDialog(this.options, this.attrs, { ...this.tooltip, pos: changes.mapPos(this.tooltip.pos) }, this.timestamp, this.selected, this.disabled);
    }
    setDisabled() {
        return new CompletionDialog(this.options, this.attrs, this.tooltip, this.timestamp, this.selected, true);
    }
}
class CompletionState {
    constructor(active, id, open) {
        this.active = active;
        this.id = id;
        this.open = open;
    }
    static start() {
        return new CompletionState(none, "cm-ac-" + Math.floor(Math.random() * 2e6).toString(36), null);
    }
    update(tr) {
        let { state } = tr, conf = state.facet(completionConfig);
        let sources = conf.override ||
            state.languageDataAt("autocomplete", cur(state)).map(asSource);
        let active = sources.map(source => {
            let value = this.active.find(s => s.source == source) ||
                new ActiveSource(source, this.active.some(a => a.state != 0 /* State.Inactive */) ? 1 /* State.Pending */ : 0 /* State.Inactive */);
            return value.update(tr, conf);
        });
        if (active.length == this.active.length && active.every((a, i) => a == this.active[i]))
            active = this.active;
        let open = this.open, didSet = tr.effects.some(e => e.is(setActiveEffect));
        if (open && tr.docChanged)
            open = open.map(tr.changes);
        if (tr.selection || active.some(a => a.hasResult() && tr.changes.touchesRange(a.from, a.to)) ||
            !sameResults(active, this.active) || didSet)
            open = CompletionDialog.build(active, state, this.id, open, conf, didSet);
        else if (open && open.disabled && !active.some(a => a.isPending))
            open = null;
        if (!open && active.every(a => !a.isPending) && active.some(a => a.hasResult()))
            active = active.map(a => a.hasResult() ? new ActiveSource(a.source, 0 /* State.Inactive */) : a);
        for (let effect of tr.effects)
            if (effect.is(setSelectedEffect))
                open = open && open.setSelected(effect.value, this.id);
        return active == this.active && open == this.open ? this : new CompletionState(active, this.id, open);
    }
    get tooltip() { return this.open ? this.open.tooltip : null; }
    get attrs() { return this.open ? this.open.attrs : this.active.length ? baseAttrs : noAttrs; }
}
function sameResults(a, b) {
    if (a == b)
        return true;
    for (let iA = 0, iB = 0;;) {
        while (iA < a.length && !a[iA].hasResult())
            iA++;
        while (iB < b.length && !b[iB].hasResult())
            iB++;
        let endA = iA == a.length, endB = iB == b.length;
        if (endA || endB)
            return endA == endB;
        if (a[iA++].result != b[iB++].result)
            return false;
    }
}
const baseAttrs = {
    "aria-autocomplete": "list"
};
const noAttrs = {};
function makeAttrs(id, selected) {
    let result = {
        "aria-autocomplete": "list",
        "aria-haspopup": "listbox",
        "aria-controls": id
    };
    if (selected > -1)
        result["aria-activedescendant"] = id + "-" + selected;
    return result;
}
const none = [];
function getUpdateType(tr, conf) {
    if (tr.isUserEvent("input.complete")) {
        let completion = tr.annotation(pickedCompletion);
        if (completion && conf.activateOnCompletion(completion))
            return 4 /* UpdateType.Activate */ | 8 /* UpdateType.Reset */;
    }
    let typing = tr.isUserEvent("input.type");
    return typing && conf.activateOnTyping ? 4 /* UpdateType.Activate */ | 1 /* UpdateType.Typing */
        : typing ? 1 /* UpdateType.Typing */
            : tr.isUserEvent("delete.backward") ? 2 /* UpdateType.Backspacing */
                : tr.selection ? 8 /* UpdateType.Reset */
                    : tr.docChanged ? 16 /* UpdateType.ResetIfTouching */ : 0 /* UpdateType.None */;
}
class ActiveSource {
    constructor(source, state, explicit = false) {
        this.source = source;
        this.state = state;
        this.explicit = explicit;
    }
    hasResult() { return false; }
    get isPending() { return this.state == 1 /* State.Pending */; }
    update(tr, conf) {
        let type = getUpdateType(tr, conf), value = this;
        if ((type & 8 /* UpdateType.Reset */) || (type & 16 /* UpdateType.ResetIfTouching */) && this.touches(tr))
            value = new ActiveSource(value.source, 0 /* State.Inactive */);
        if ((type & 4 /* UpdateType.Activate */) && value.state == 0 /* State.Inactive */)
            value = new ActiveSource(this.source, 1 /* State.Pending */);
        value = value.updateFor(tr, type);
        for (let effect of tr.effects) {
            if (effect.is(startCompletionEffect))
                value = new ActiveSource(value.source, 1 /* State.Pending */, effect.value);
            else if (effect.is(closeCompletionEffect))
                value = new ActiveSource(value.source, 0 /* State.Inactive */);
            else if (effect.is(setActiveEffect))
                for (let active of effect.value)
                    if (active.source == value.source)
                        value = active;
        }
        return value;
    }
    updateFor(tr, type) { return this.map(tr.changes); }
    map(changes) { return this; }
    touches(tr) {
        return tr.changes.touchesRange(cur(tr.state));
    }
}
class ActiveResult extends ActiveSource {
    constructor(source, explicit, limit, result, from, to) {
        super(source, 3 /* State.Result */, explicit);
        this.limit = limit;
        this.result = result;
        this.from = from;
        this.to = to;
    }
    hasResult() { return true; }
    updateFor(tr, type) {
        var _a;
        if (!(type & 3 /* UpdateType.SimpleInteraction */))
            return this.map(tr.changes);
        let result = this.result;
        if (result.map && !tr.changes.empty)
            result = result.map(result, tr.changes);
        let from = tr.changes.mapPos(this.from), to = tr.changes.mapPos(this.to, 1);
        let pos = cur(tr.state);
        if (pos > to || !result ||
            (type & 2 /* UpdateType.Backspacing */) && (cur(tr.startState) == this.from || pos < this.limit))
            return new ActiveSource(this.source, type & 4 /* UpdateType.Activate */ ? 1 /* State.Pending */ : 0 /* State.Inactive */);
        let limit = tr.changes.mapPos(this.limit);
        if (checkValid(result.validFor, tr.state, from, to))
            return new ActiveResult(this.source, this.explicit, limit, result, from, to);
        if (result.update &&
            (result = result.update(result, from, to, new CompletionContext(tr.state, pos, false))))
            return new ActiveResult(this.source, this.explicit, limit, result, result.from, (_a = result.to) !== null && _a !== void 0 ? _a : cur(tr.state));
        return new ActiveSource(this.source, 1 /* State.Pending */, this.explicit);
    }
    map(mapping) {
        if (mapping.empty)
            return this;
        let result = this.result.map ? this.result.map(this.result, mapping) : this.result;
        if (!result)
            return new ActiveSource(this.source, 0 /* State.Inactive */);
        return new ActiveResult(this.source, this.explicit, mapping.mapPos(this.limit), this.result, mapping.mapPos(this.from), mapping.mapPos(this.to, 1));
    }
    touches(tr) {
        return tr.changes.touchesRange(this.from, this.to);
    }
}
function checkValid(validFor, state, from, to) {
    if (!validFor)
        return false;
    let text = state.sliceDoc(from, to);
    return typeof validFor == "function" ? validFor(text, from, to, state) : ensureAnchor(validFor, true).test(text);
}
const setActiveEffect = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .StateEffect */ .Pe.define({
    map(sources, mapping) { return sources.map(s => s.map(mapping)); }
});
const completionState = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .StateField */ .sU.define({
    create() { return CompletionState.start(); },
    update(value, tr) { return value.update(tr); },
    provide: f => [
        _codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .showTooltip */ .DK.from(f, val => val.tooltip),
        _codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .EditorView */ .Lz.contentAttributes.from(f, state => state.attrs)
    ]
});
function applyCompletion(view, option) {
    const apply = option.completion.apply || option.completion.label;
    let result = view.state.field(completionState).active.find(a => a.source == option.source);
    if (!(result instanceof ActiveResult))
        return false;
    if (typeof apply == "string")
        view.dispatch({
            ...insertCompletionText(view.state, apply, result.from, result.to),
            annotations: pickedCompletion.of(option.completion)
        });
    else
        apply(view, option.completion, result.from, result.to);
    return true;
}
const createTooltip = /*@__PURE__*/completionTooltip(completionState, applyCompletion);

/**
Returns a command that moves the completion selection forward or
backward by the given amount.
*/
function moveCompletionSelection(forward, by = "option") {
    return (view) => {
        let cState = view.state.field(completionState, false);
        if (!cState || !cState.open || cState.open.disabled ||
            Date.now() - cState.open.timestamp < view.state.facet(completionConfig).interactionDelay)
            return false;
        let step = 1, tooltip;
        if (by == "page" && (tooltip = (0,_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .getTooltip */ .Eg)(view, cState.open.tooltip)))
            step = Math.max(2, Math.floor(tooltip.dom.offsetHeight /
                tooltip.dom.querySelector("li").offsetHeight) - 1);
        let { length } = cState.open.options;
        let selected = cState.open.selected > -1 ? cState.open.selected + step * (forward ? 1 : -1) : forward ? 0 : length - 1;
        if (selected < 0)
            selected = by == "page" ? 0 : length - 1;
        else if (selected >= length)
            selected = by == "page" ? length - 1 : 0;
        view.dispatch({ effects: setSelectedEffect.of(selected) });
        return true;
    };
}
/**
Accept the current completion.
*/
const acceptCompletion = (view) => {
    let cState = view.state.field(completionState, false);
    if (view.state.readOnly || !cState || !cState.open || cState.open.selected < 0 || cState.open.disabled ||
        Date.now() - cState.open.timestamp < view.state.facet(completionConfig).interactionDelay)
        return false;
    return applyCompletion(view, cState.open.options[cState.open.selected]);
};
/**
Explicitly start autocompletion.
*/
const startCompletion = (view) => {
    let cState = view.state.field(completionState, false);
    if (!cState)
        return false;
    view.dispatch({ effects: startCompletionEffect.of(true) });
    return true;
};
/**
Close the currently active completion.
*/
const closeCompletion = (view) => {
    let cState = view.state.field(completionState, false);
    if (!cState || !cState.active.some(a => a.state != 0 /* State.Inactive */))
        return false;
    view.dispatch({ effects: closeCompletionEffect.of(null) });
    return true;
};
class RunningQuery {
    constructor(active, context) {
        this.active = active;
        this.context = context;
        this.time = Date.now();
        this.updates = [];
        // Note that 'undefined' means 'not done yet', whereas 'null' means
        // 'query returned null'.
        this.done = undefined;
    }
}
const MaxUpdateCount = 50, MinAbortTime = 1000;
const completionPlugin = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .ViewPlugin */ .Z9.fromClass(class {
    constructor(view) {
        this.view = view;
        this.debounceUpdate = -1;
        this.running = [];
        this.debounceAccept = -1;
        this.pendingStart = false;
        this.composing = 0 /* CompositionState.None */;
        for (let active of view.state.field(completionState).active)
            if (active.isPending)
                this.startQuery(active);
    }
    update(update) {
        let cState = update.state.field(completionState);
        let conf = update.state.facet(completionConfig);
        if (!update.selectionSet && !update.docChanged && update.startState.field(completionState) == cState)
            return;
        let doesReset = update.transactions.some(tr => {
            let type = getUpdateType(tr, conf);
            return (type & 8 /* UpdateType.Reset */) || (tr.selection || tr.docChanged) && !(type & 3 /* UpdateType.SimpleInteraction */);
        });
        for (let i = 0; i < this.running.length; i++) {
            let query = this.running[i];
            if (doesReset ||
                query.context.abortOnDocChange && update.docChanged ||
                query.updates.length + update.transactions.length > MaxUpdateCount && Date.now() - query.time > MinAbortTime) {
                for (let handler of query.context.abortListeners) {
                    try {
                        handler();
                    }
                    catch (e) {
                        (0,_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .logException */ .c_)(this.view.state, e);
                    }
                }
                query.context.abortListeners = null;
                this.running.splice(i--, 1);
            }
            else {
                query.updates.push(...update.transactions);
            }
        }
        if (this.debounceUpdate > -1)
            clearTimeout(this.debounceUpdate);
        if (update.transactions.some(tr => tr.effects.some(e => e.is(startCompletionEffect))))
            this.pendingStart = true;
        let delay = this.pendingStart ? 50 : conf.activateOnTypingDelay;
        this.debounceUpdate = cState.active.some(a => a.isPending && !this.running.some(q => q.active.source == a.source))
            ? setTimeout(() => this.startUpdate(), delay) : -1;
        if (this.composing != 0 /* CompositionState.None */)
            for (let tr of update.transactions) {
                if (tr.isUserEvent("input.type"))
                    this.composing = 2 /* CompositionState.Changed */;
                else if (this.composing == 2 /* CompositionState.Changed */ && tr.selection)
                    this.composing = 3 /* CompositionState.ChangedAndMoved */;
            }
    }
    startUpdate() {
        this.debounceUpdate = -1;
        this.pendingStart = false;
        let { state } = this.view, cState = state.field(completionState);
        for (let active of cState.active) {
            if (active.isPending && !this.running.some(r => r.active.source == active.source))
                this.startQuery(active);
        }
        if (this.running.length && cState.open && cState.open.disabled)
            this.debounceAccept = setTimeout(() => this.accept(), this.view.state.facet(completionConfig).updateSyncTime);
    }
    startQuery(active) {
        let { state } = this.view, pos = cur(state);
        let context = new CompletionContext(state, pos, active.explicit, this.view);
        let pending = new RunningQuery(active, context);
        this.running.push(pending);
        Promise.resolve(active.source(context)).then(result => {
            if (!pending.context.aborted) {
                pending.done = result || null;
                this.scheduleAccept();
            }
        }, err => {
            this.view.dispatch({ effects: closeCompletionEffect.of(null) });
            (0,_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .logException */ .c_)(this.view.state, err);
        });
    }
    scheduleAccept() {
        if (this.running.every(q => q.done !== undefined))
            this.accept();
        else if (this.debounceAccept < 0)
            this.debounceAccept = setTimeout(() => this.accept(), this.view.state.facet(completionConfig).updateSyncTime);
    }
    // For each finished query in this.running, try to create a result
    // or, if appropriate, restart the query.
    accept() {
        var _a;
        if (this.debounceAccept > -1)
            clearTimeout(this.debounceAccept);
        this.debounceAccept = -1;
        let updated = [];
        let conf = this.view.state.facet(completionConfig), cState = this.view.state.field(completionState);
        for (let i = 0; i < this.running.length; i++) {
            let query = this.running[i];
            if (query.done === undefined)
                continue;
            this.running.splice(i--, 1);
            if (query.done) {
                let pos = cur(query.updates.length ? query.updates[0].startState : this.view.state);
                let limit = Math.min(pos, query.done.from + (query.active.explicit ? 0 : 1));
                let active = new ActiveResult(query.active.source, query.active.explicit, limit, query.done, query.done.from, (_a = query.done.to) !== null && _a !== void 0 ? _a : pos);
                // Replay the transactions that happened since the start of
                // the request and see if that preserves the result
                for (let tr of query.updates)
                    active = active.update(tr, conf);
                if (active.hasResult()) {
                    updated.push(active);
                    continue;
                }
            }
            let current = cState.active.find(a => a.source == query.active.source);
            if (current && current.isPending) {
                if (query.done == null) {
                    // Explicitly failed. Should clear the pending status if it
                    // hasn't been re-set in the meantime.
                    let active = new ActiveSource(query.active.source, 0 /* State.Inactive */);
                    for (let tr of query.updates)
                        active = active.update(tr, conf);
                    if (!active.isPending)
                        updated.push(active);
                }
                else {
                    // Cleared by subsequent transactions. Restart.
                    this.startQuery(current);
                }
            }
        }
        if (updated.length || cState.open && cState.open.disabled)
            this.view.dispatch({ effects: setActiveEffect.of(updated) });
    }
}, {
    eventHandlers: {
        blur(event) {
            let state = this.view.state.field(completionState, false);
            if (state && state.tooltip && this.view.state.facet(completionConfig).closeOnBlur) {
                let dialog = state.open && (0,_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .getTooltip */ .Eg)(this.view, state.open.tooltip);
                if (!dialog || !dialog.dom.contains(event.relatedTarget))
                    setTimeout(() => this.view.dispatch({ effects: closeCompletionEffect.of(null) }), 10);
            }
        },
        compositionstart() {
            this.composing = 1 /* CompositionState.Started */;
        },
        compositionend() {
            if (this.composing == 3 /* CompositionState.ChangedAndMoved */) {
                // Safari fires compositionend events synchronously, possibly
                // from inside an update, so dispatch asynchronously to avoid reentrancy
                setTimeout(() => this.view.dispatch({ effects: startCompletionEffect.of(false) }), 20);
            }
            this.composing = 0 /* CompositionState.None */;
        }
    }
});
const windows = typeof navigator == "object" && /*@__PURE__*//Win/.test(navigator.platform);
const commitCharacters = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .Prec */ .Nb.highest(/*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .EditorView */ .Lz.domEventHandlers({
    keydown(event, view) {
        let field = view.state.field(completionState, false);
        if (!field || !field.open || field.open.disabled || field.open.selected < 0 ||
            event.key.length > 1 || event.ctrlKey && !(windows && event.altKey) || event.metaKey)
            return false;
        let option = field.open.options[field.open.selected];
        let result = field.active.find(a => a.source == option.source);
        let commitChars = option.completion.commitCharacters || result.result.commitCharacters;
        if (commitChars && commitChars.indexOf(event.key) > -1)
            applyCompletion(view, option);
        return false;
    }
}));

const baseTheme = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .EditorView */ .Lz.baseTheme({
    ".cm-tooltip.cm-tooltip-autocomplete": {
        "& > ul": {
            fontFamily: "monospace",
            whiteSpace: "nowrap",
            overflow: "hidden auto",
            maxWidth_fallback: "700px",
            maxWidth: "min(700px, 95vw)",
            minWidth: "250px",
            maxHeight: "10em",
            height: "100%",
            listStyle: "none",
            margin: 0,
            padding: 0,
            "& > li, & > completion-section": {
                padding: "1px 3px",
                lineHeight: 1.2
            },
            "& > li": {
                overflowX: "hidden",
                textOverflow: "ellipsis",
                cursor: "pointer"
            },
            "& > completion-section": {
                display: "list-item",
                borderBottom: "1px solid silver",
                paddingLeft: "0.5em",
                opacity: 0.7
            }
        }
    },
    "&light .cm-tooltip-autocomplete ul li[aria-selected]": {
        background: "#17c",
        color: "white",
    },
    "&light .cm-tooltip-autocomplete-disabled ul li[aria-selected]": {
        background: "#777",
    },
    "&dark .cm-tooltip-autocomplete ul li[aria-selected]": {
        background: "#347",
        color: "white",
    },
    "&dark .cm-tooltip-autocomplete-disabled ul li[aria-selected]": {
        background: "#444",
    },
    ".cm-completionListIncompleteTop:before, .cm-completionListIncompleteBottom:after": {
        content: '"···"',
        opacity: 0.5,
        display: "block",
        textAlign: "center"
    },
    ".cm-tooltip.cm-completionInfo": {
        position: "absolute",
        padding: "3px 9px",
        width: "max-content",
        maxWidth: `${400 /* Info.Width */}px`,
        boxSizing: "border-box",
        whiteSpace: "pre-line"
    },
    ".cm-completionInfo.cm-completionInfo-left": { right: "100%" },
    ".cm-completionInfo.cm-completionInfo-right": { left: "100%" },
    ".cm-completionInfo.cm-completionInfo-left-narrow": { right: `${30 /* Info.Margin */}px` },
    ".cm-completionInfo.cm-completionInfo-right-narrow": { left: `${30 /* Info.Margin */}px` },
    "&light .cm-snippetField": { backgroundColor: "#00000022" },
    "&dark .cm-snippetField": { backgroundColor: "#ffffff22" },
    ".cm-snippetFieldPosition": {
        verticalAlign: "text-top",
        width: 0,
        height: "1.15em",
        display: "inline-block",
        margin: "0 -0.7px -.7em",
        borderLeft: "1.4px dotted #888"
    },
    ".cm-completionMatchedText": {
        textDecoration: "underline"
    },
    ".cm-completionDetail": {
        marginLeft: "0.5em",
        fontStyle: "italic"
    },
    ".cm-completionIcon": {
        fontSize: "90%",
        width: ".8em",
        display: "inline-block",
        textAlign: "center",
        paddingRight: ".6em",
        opacity: "0.6",
        boxSizing: "content-box"
    },
    ".cm-completionIcon-function, .cm-completionIcon-method": {
        "&:after": { content: "'ƒ'" }
    },
    ".cm-completionIcon-class": {
        "&:after": { content: "'○'" }
    },
    ".cm-completionIcon-interface": {
        "&:after": { content: "'◌'" }
    },
    ".cm-completionIcon-variable": {
        "&:after": { content: "'𝑥'" }
    },
    ".cm-completionIcon-constant": {
        "&:after": { content: "'𝐶'" }
    },
    ".cm-completionIcon-type": {
        "&:after": { content: "'𝑡'" }
    },
    ".cm-completionIcon-enum": {
        "&:after": { content: "'∪'" }
    },
    ".cm-completionIcon-property": {
        "&:after": { content: "'□'" }
    },
    ".cm-completionIcon-keyword": {
        "&:after": { content: "'🔑\uFE0E'" } // Disable emoji rendering
    },
    ".cm-completionIcon-namespace": {
        "&:after": { content: "'▢'" }
    },
    ".cm-completionIcon-text": {
        "&:after": { content: "'abc'", fontSize: "50%", verticalAlign: "middle" }
    }
});

class FieldPos {
    constructor(field, line, from, to) {
        this.field = field;
        this.line = line;
        this.from = from;
        this.to = to;
    }
}
class FieldRange {
    constructor(field, from, to) {
        this.field = field;
        this.from = from;
        this.to = to;
    }
    map(changes) {
        let from = changes.mapPos(this.from, -1, _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .MapMode */ .iR.TrackDel);
        let to = changes.mapPos(this.to, 1, _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .MapMode */ .iR.TrackDel);
        return from == null || to == null ? null : new FieldRange(this.field, from, to);
    }
}
class Snippet {
    constructor(lines, fieldPositions) {
        this.lines = lines;
        this.fieldPositions = fieldPositions;
    }
    instantiate(state, pos) {
        let text = [], lineStart = [pos];
        let lineObj = state.doc.lineAt(pos), baseIndent = /^\s*/.exec(lineObj.text)[0];
        for (let line of this.lines) {
            if (text.length) {
                let indent = baseIndent, tabs = /^\t*/.exec(line)[0].length;
                for (let i = 0; i < tabs; i++)
                    indent += state.facet(_codemirror_language__WEBPACK_IMPORTED_MODULE_0__/* .indentUnit */ .Xt);
                lineStart.push(pos + indent.length - tabs);
                line = indent + line.slice(tabs);
            }
            text.push(line);
            pos += line.length + 1;
        }
        let ranges = this.fieldPositions.map(pos => new FieldRange(pos.field, lineStart[pos.line] + pos.from, lineStart[pos.line] + pos.to));
        return { text, ranges };
    }
    static parse(template) {
        let fields = [];
        let lines = [], positions = [], m;
        for (let line of template.split(/\r\n?|\n/)) {
            while (m = /[#$]\{(?:(\d+)(?::([^{}]*))?|((?:\\[{}]|[^{}])*))\}/.exec(line)) {
                let seq = m[1] ? +m[1] : null, rawName = m[2] || m[3] || "", found = -1;
                let name = rawName.replace(/\\[{}]/g, m => m[1]);
                for (let i = 0; i < fields.length; i++) {
                    if (seq != null ? fields[i].seq == seq : name ? fields[i].name == name : false)
                        found = i;
                }
                if (found < 0) {
                    let i = 0;
                    while (i < fields.length && (seq == null || (fields[i].seq != null && fields[i].seq < seq)))
                        i++;
                    fields.splice(i, 0, { seq, name });
                    found = i;
                    for (let pos of positions)
                        if (pos.field >= found)
                            pos.field++;
                }
                for (let pos of positions)
                    if (pos.line == lines.length && pos.from > m.index) {
                        let snip = m[2] ? 3 + (m[1] || "").length : 2;
                        pos.from -= snip;
                        pos.to -= snip;
                    }
                positions.push(new FieldPos(found, lines.length, m.index, m.index + name.length));
                line = line.slice(0, m.index) + rawName + line.slice(m.index + m[0].length);
            }
            line = line.replace(/\\([{}])/g, (_, brace, index) => {
                for (let pos of positions)
                    if (pos.line == lines.length && pos.from > index) {
                        pos.from--;
                        pos.to--;
                    }
                return brace;
            });
            lines.push(line);
        }
        return new Snippet(lines, positions);
    }
}
let fieldMarker = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .Decoration */ .NZ.widget({ widget: /*@__PURE__*/new class extends _codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .WidgetType */ .xO {
        toDOM() {
            let span = document.createElement("span");
            span.className = "cm-snippetFieldPosition";
            return span;
        }
        ignoreEvent() { return false; }
    } });
let fieldRange = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .Decoration */ .NZ.mark({ class: "cm-snippetField" });
class ActiveSnippet {
    constructor(ranges, active) {
        this.ranges = ranges;
        this.active = active;
        this.deco = _codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .Decoration */ .NZ.set(ranges.map(r => (r.from == r.to ? fieldMarker : fieldRange).range(r.from, r.to)), true);
    }
    map(changes) {
        let ranges = [];
        for (let r of this.ranges) {
            let mapped = r.map(changes);
            if (!mapped)
                return null;
            ranges.push(mapped);
        }
        return new ActiveSnippet(ranges, this.active);
    }
    selectionInsideField(sel) {
        return sel.ranges.every(range => this.ranges.some(r => r.field == this.active && r.from <= range.from && r.to >= range.to));
    }
}
const setActive = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .StateEffect */ .Pe.define({
    map(value, changes) { return value && value.map(changes); }
});
const moveToField = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .StateEffect */ .Pe.define();
const snippetState = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .StateField */ .sU.define({
    create() { return null; },
    update(value, tr) {
        for (let effect of tr.effects) {
            if (effect.is(setActive))
                return effect.value;
            if (effect.is(moveToField) && value)
                return new ActiveSnippet(value.ranges, effect.value);
        }
        if (value && tr.docChanged)
            value = value.map(tr.changes);
        if (value && tr.selection && !value.selectionInsideField(tr.selection))
            value = null;
        return value;
    },
    provide: f => _codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .EditorView */ .Lz.decorations.from(f, val => val ? val.deco : _codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .Decoration */ .NZ.none)
});
function fieldSelection(ranges, field) {
    return _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .EditorSelection */ .OF.create(ranges.filter(r => r.field == field).map(r => _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .EditorSelection */ .OF.range(r.from, r.to)));
}
/**
Convert a snippet template to a function that can
[apply](https://codemirror.net/6/docs/ref/#autocomplete.Completion.apply) it. Snippets are written
using syntax like this:

    "for (let ${index} = 0; ${index} < ${end}; ${index}++) {\n\t${}\n}"

Each `${}` placeholder (you may also use `#{}`) indicates a field
that the user can fill in. Its name, if any, will be the default
content for the field.

When the snippet is activated by calling the returned function,
the code is inserted at the given position. Newlines in the
template are indented by the indentation of the start line, plus
one [indent unit](https://codemirror.net/6/docs/ref/#language.indentUnit) per tab character after
the newline.

On activation, (all instances of) the first field are selected.
The user can move between fields with Tab and Shift-Tab as long as
the fields are active. Moving to the last field or moving the
cursor out of the current field deactivates the fields.

The order of fields defaults to textual order, but you can add
numbers to placeholders (`${1}` or `${1:defaultText}`) to provide
a custom order.

To include a literal `{` or `}` in your template, put a backslash
in front of it. This will be removed and the brace will not be
interpreted as indicating a placeholder.
*/
function snippet(template) {
    let snippet = Snippet.parse(template);
    return (editor, completion, from, to) => {
        let { text, ranges } = snippet.instantiate(editor.state, from);
        let { main } = editor.state.selection;
        let spec = {
            changes: { from, to: to == main.from ? main.to : to, insert: _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .Text */ .EY.of(text) },
            scrollIntoView: true,
            annotations: completion ? [pickedCompletion.of(completion), _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .Transaction */ .ZX.userEvent.of("input.complete")] : undefined
        };
        if (ranges.length)
            spec.selection = fieldSelection(ranges, 0);
        if (ranges.some(r => r.field > 0)) {
            let active = new ActiveSnippet(ranges, 0);
            let effects = spec.effects = [setActive.of(active)];
            if (editor.state.field(snippetState, false) === undefined)
                effects.push(_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .StateEffect */ .Pe.appendConfig.of([snippetState, addSnippetKeymap, snippetPointerHandler, baseTheme]));
        }
        editor.dispatch(editor.state.update(spec));
    };
}
function moveField(dir) {
    return ({ state, dispatch }) => {
        let active = state.field(snippetState, false);
        if (!active || dir < 0 && active.active == 0)
            return false;
        let next = active.active + dir, last = dir > 0 && !active.ranges.some(r => r.field == next + dir);
        dispatch(state.update({
            selection: fieldSelection(active.ranges, next),
            effects: setActive.of(last ? null : new ActiveSnippet(active.ranges, next)),
            scrollIntoView: true
        }));
        return true;
    };
}
/**
A command that clears the active snippet, if any.
*/
const clearSnippet = ({ state, dispatch }) => {
    let active = state.field(snippetState, false);
    if (!active)
        return false;
    dispatch(state.update({ effects: setActive.of(null) }));
    return true;
};
/**
Move to the next snippet field, if available.
*/
const nextSnippetField = /*@__PURE__*/moveField(1);
/**
Move to the previous snippet field, if available.
*/
const prevSnippetField = /*@__PURE__*/moveField(-1);
/**
Check if there is an active snippet with a next field for
`nextSnippetField` to move to.
*/
function hasNextSnippetField(state) {
    let active = state.field(snippetState, false);
    return !!(active && active.ranges.some(r => r.field == active.active + 1));
}
/**
Returns true if there is an active snippet and a previous field
for `prevSnippetField` to move to.
*/
function hasPrevSnippetField(state) {
    let active = state.field(snippetState, false);
    return !!(active && active.active > 0);
}
const defaultSnippetKeymap = [
    { key: "Tab", run: nextSnippetField, shift: prevSnippetField },
    { key: "Escape", run: clearSnippet }
];
/**
A facet that can be used to configure the key bindings used by
snippets. The default binds Tab to
[`nextSnippetField`](https://codemirror.net/6/docs/ref/#autocomplete.nextSnippetField), Shift-Tab to
[`prevSnippetField`](https://codemirror.net/6/docs/ref/#autocomplete.prevSnippetField), and Escape
to [`clearSnippet`](https://codemirror.net/6/docs/ref/#autocomplete.clearSnippet).
*/
const snippetKeymap = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .Facet */ .sj.define({
    combine(maps) { return maps.length ? maps[0] : defaultSnippetKeymap; }
});
const addSnippetKeymap = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .Prec */ .Nb.highest(/*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .keymap */ .w4.compute([snippetKeymap], state => state.facet(snippetKeymap)));
/**
Create a completion from a snippet. Returns an object with the
properties from `completion`, plus an `apply` function that
applies the snippet.
*/
function snippetCompletion(template, completion) {
    return { ...completion, apply: snippet(template) };
}
const snippetPointerHandler = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .EditorView */ .Lz.domEventHandlers({
    mousedown(event, view) {
        let active = view.state.field(snippetState, false), pos;
        if (!active || (pos = view.posAtCoords({ x: event.clientX, y: event.clientY })) == null)
            return false;
        let match = active.ranges.find(r => r.from <= pos && r.to >= pos);
        if (!match || match.field == active.active)
            return false;
        view.dispatch({
            selection: fieldSelection(active.ranges, match.field),
            effects: setActive.of(active.ranges.some(r => r.field > match.field)
                ? new ActiveSnippet(active.ranges, match.field) : null),
            scrollIntoView: true
        });
        return true;
    }
});

function wordRE(wordChars) {
    let escaped = wordChars.replace(/[\]\-\\]/g, "\\$&");
    try {
        return new RegExp(`[\\p{Alphabetic}\\p{Number}_${escaped}]+`, "ug");
    }
    catch (_a) {
        return new RegExp(`[\w${escaped}]`, "g");
    }
}
function mapRE(re, f) {
    return new RegExp(f(re.source), re.unicode ? "u" : "");
}
const wordCaches = /*@__PURE__*/(/* unused pure expression or super */ null && (Object.create(null)));
function wordCache(wordChars) {
    return wordCaches[wordChars] || (wordCaches[wordChars] = new WeakMap);
}
function storeWords(doc, wordRE, result, seen, ignoreAt) {
    for (let lines = doc.iterLines(), pos = 0; !lines.next().done;) {
        let { value } = lines, m;
        wordRE.lastIndex = 0;
        while (m = wordRE.exec(value)) {
            if (!seen[m[0]] && pos + m.index != ignoreAt) {
                result.push({ type: "text", label: m[0] });
                seen[m[0]] = true;
                if (result.length >= 2000 /* C.MaxList */)
                    return;
            }
        }
        pos += value.length + 1;
    }
}
function collectWords(doc, cache, wordRE, to, ignoreAt) {
    let big = doc.length >= 1000 /* C.MinCacheLen */;
    let cached = big && cache.get(doc);
    if (cached)
        return cached;
    let result = [], seen = Object.create(null);
    if (doc.children) {
        let pos = 0;
        for (let ch of doc.children) {
            if (ch.length >= 1000 /* C.MinCacheLen */) {
                for (let c of collectWords(ch, cache, wordRE, to - pos, ignoreAt - pos)) {
                    if (!seen[c.label]) {
                        seen[c.label] = true;
                        result.push(c);
                    }
                }
            }
            else {
                storeWords(ch, wordRE, result, seen, ignoreAt - pos);
            }
            pos += ch.length + 1;
        }
    }
    else {
        storeWords(doc, wordRE, result, seen, ignoreAt);
    }
    if (big && result.length < 2000 /* C.MaxList */)
        cache.set(doc, result);
    return result;
}
/**
A completion source that will scan the document for words (using a
[character categorizer](https://codemirror.net/6/docs/ref/#state.EditorState.charCategorizer)), and
return those as completions.
*/
const completeAnyWord = context => {
    var _a;
    let wordChars = (_a = context.state.languageDataAt("wordChars", context.pos)[0]) !== null && _a !== void 0 ? _a : "";
    let re = wordRE(wordChars);
    let token = context.matchBefore(mapRE(re, s => s + "$"));
    if (!token && !context.explicit)
        return null;
    let from = token ? token.from : context.pos;
    let options = collectWords(context.state.doc, wordCache(wordChars), re, 50000 /* C.Range */, from);
    return { from, options, validFor: mapRE(re, s => "^" + s) };
};

const defaults = {
    brackets: ["(", "[", "{", "'", '"'],
    before: ")]}:;>",
    stringPrefixes: []
};
const closeBracketEffect = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .StateEffect */ .Pe.define({
    map(value, mapping) {
        let mapped = mapping.mapPos(value, -1, _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .MapMode */ .iR.TrackAfter);
        return mapped == null ? undefined : mapped;
    }
});
const closedBracket = /*@__PURE__*/new class extends _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .RangeValue */ .FB {
};
closedBracket.startSide = 1;
closedBracket.endSide = -1;
const bracketState = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .StateField */ .sU.define({
    create() { return _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .RangeSet */ .om.empty; },
    update(value, tr) {
        value = value.map(tr.changes);
        if (tr.selection) {
            let line = tr.state.doc.lineAt(tr.selection.main.head);
            value = value.update({ filter: from => from >= line.from && from <= line.to });
        }
        for (let effect of tr.effects)
            if (effect.is(closeBracketEffect))
                value = value.update({ add: [closedBracket.range(effect.value, effect.value + 1)] });
        return value;
    }
});
/**
Extension to enable bracket-closing behavior. When a closeable
bracket is typed, its closing bracket is immediately inserted
after the cursor. When closing a bracket directly in front of a
closing bracket inserted by the extension, the cursor moves over
that bracket.
*/
function closeBrackets() {
    return [inputHandler, bracketState];
}
const definedClosing = "()[]{}<>«»»«［］｛｝";
function closing(ch) {
    for (let i = 0; i < definedClosing.length; i += 2)
        if (definedClosing.charCodeAt(i) == ch)
            return definedClosing.charAt(i + 1);
    return (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .fromCodePoint */ .MK)(ch < 128 ? ch : ch + 1);
}
function config(state, pos) {
    return state.languageDataAt("closeBrackets", pos)[0] || defaults;
}
const android = typeof navigator == "object" && /*@__PURE__*//Android\b/.test(navigator.userAgent);
const inputHandler = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .EditorView */ .Lz.inputHandler.of((view, from, to, insert) => {
    if ((android ? view.composing : view.compositionStarted) || view.state.readOnly)
        return false;
    let sel = view.state.selection.main;
    if (insert.length > 2 || insert.length == 2 && (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointSize */ .Fh)((0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointAt */ .vS)(insert, 0)) == 1 ||
        from != sel.from || to != sel.to)
        return false;
    let tr = insertBracket(view.state, insert);
    if (!tr)
        return false;
    view.dispatch(tr);
    return true;
});
/**
Command that implements deleting a pair of matching brackets when
the cursor is between them.
*/
const deleteBracketPair = ({ state, dispatch }) => {
    if (state.readOnly)
        return false;
    let conf = config(state, state.selection.main.head);
    let tokens = conf.brackets || defaults.brackets;
    let dont = null, changes = state.changeByRange(range => {
        if (range.empty) {
            let before = prevChar(state.doc, range.head);
            for (let token of tokens) {
                if (token == before && nextChar(state.doc, range.head) == closing((0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointAt */ .vS)(token, 0)))
                    return { changes: { from: range.head - token.length, to: range.head + token.length },
                        range: _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .EditorSelection */ .OF.cursor(range.head - token.length) };
            }
        }
        return { range: dont = range };
    });
    if (!dont)
        dispatch(state.update(changes, { scrollIntoView: true, userEvent: "delete.backward" }));
    return !dont;
};
/**
Close-brackets related key bindings. Binds Backspace to
[`deleteBracketPair`](https://codemirror.net/6/docs/ref/#autocomplete.deleteBracketPair).
*/
const closeBracketsKeymap = [
    { key: "Backspace", run: deleteBracketPair }
];
/**
Implements the extension's behavior on text insertion. If the
given string counts as a bracket in the language around the
selection, and replacing the selection with it requires custom
behavior (inserting a closing version or skipping past a
previously-closed bracket), this function returns a transaction
representing that custom behavior. (You only need this if you want
to programmatically insert brackets—the
[`closeBrackets`](https://codemirror.net/6/docs/ref/#autocomplete.closeBrackets) extension will
take care of running this for user input.)
*/
function insertBracket(state, bracket) {
    let conf = config(state, state.selection.main.head);
    let tokens = conf.brackets || defaults.brackets;
    for (let tok of tokens) {
        let closed = closing((0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointAt */ .vS)(tok, 0));
        if (bracket == tok)
            return closed == tok ? handleSame(state, tok, tokens.indexOf(tok + tok + tok) > -1, conf)
                : handleOpen(state, tok, closed, conf.before || defaults.before);
        if (bracket == closed && closedBracketAt(state, state.selection.main.from))
            return handleClose(state, tok, closed);
    }
    return null;
}
function closedBracketAt(state, pos) {
    let found = false;
    state.field(bracketState).between(0, state.doc.length, from => {
        if (from == pos)
            found = true;
    });
    return found;
}
function nextChar(doc, pos) {
    let next = doc.sliceString(pos, pos + 2);
    return next.slice(0, (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointSize */ .Fh)((0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointAt */ .vS)(next, 0)));
}
function prevChar(doc, pos) {
    let prev = doc.sliceString(pos - 2, pos);
    return (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointSize */ .Fh)((0,_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .codePointAt */ .vS)(prev, 0)) == prev.length ? prev : prev.slice(1);
}
function handleOpen(state, open, close, closeBefore) {
    let dont = null, changes = state.changeByRange(range => {
        if (!range.empty)
            return { changes: [{ insert: open, from: range.from }, { insert: close, from: range.to }],
                effects: closeBracketEffect.of(range.to + open.length),
                range: _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .EditorSelection */ .OF.range(range.anchor + open.length, range.head + open.length) };
        let next = nextChar(state.doc, range.head);
        if (!next || /\s/.test(next) || closeBefore.indexOf(next) > -1)
            return { changes: { insert: open + close, from: range.head },
                effects: closeBracketEffect.of(range.head + open.length),
                range: _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .EditorSelection */ .OF.cursor(range.head + open.length) };
        return { range: dont = range };
    });
    return dont ? null : state.update(changes, {
        scrollIntoView: true,
        userEvent: "input.type"
    });
}
function handleClose(state, _open, close) {
    let dont = null, changes = state.changeByRange(range => {
        if (range.empty && nextChar(state.doc, range.head) == close)
            return { changes: { from: range.head, to: range.head + close.length, insert: close },
                range: _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .EditorSelection */ .OF.cursor(range.head + close.length) };
        return dont = { range };
    });
    return dont ? null : state.update(changes, {
        scrollIntoView: true,
        userEvent: "input.type"
    });
}
// Handles cases where the open and close token are the same, and
// possibly triple quotes (as in `"""abc"""`-style quoting).
function handleSame(state, token, allowTriple, config) {
    let stringPrefixes = config.stringPrefixes || defaults.stringPrefixes;
    let dont = null, changes = state.changeByRange(range => {
        if (!range.empty)
            return { changes: [{ insert: token, from: range.from }, { insert: token, from: range.to }],
                effects: closeBracketEffect.of(range.to + token.length),
                range: _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .EditorSelection */ .OF.range(range.anchor + token.length, range.head + token.length) };
        let pos = range.head, next = nextChar(state.doc, pos), start;
        if (next == token) {
            if (nodeStart(state, pos)) {
                return { changes: { insert: token + token, from: pos },
                    effects: closeBracketEffect.of(pos + token.length),
                    range: _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .EditorSelection */ .OF.cursor(pos + token.length) };
            }
            else if (closedBracketAt(state, pos)) {
                let isTriple = allowTriple && state.sliceDoc(pos, pos + token.length * 3) == token + token + token;
                let content = isTriple ? token + token + token : token;
                return { changes: { from: pos, to: pos + content.length, insert: content },
                    range: _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .EditorSelection */ .OF.cursor(pos + content.length) };
            }
        }
        else if (allowTriple && state.sliceDoc(pos - 2 * token.length, pos) == token + token &&
            (start = canStartStringAt(state, pos - 2 * token.length, stringPrefixes)) > -1 &&
            nodeStart(state, start)) {
            return { changes: { insert: token + token + token + token, from: pos },
                effects: closeBracketEffect.of(pos + token.length),
                range: _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .EditorSelection */ .OF.cursor(pos + token.length) };
        }
        else if (state.charCategorizer(pos)(next) != _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .CharCategory */ .Je.Word) {
            if (canStartStringAt(state, pos, stringPrefixes) > -1 && !probablyInString(state, pos, token, stringPrefixes))
                return { changes: { insert: token + token, from: pos },
                    effects: closeBracketEffect.of(pos + token.length),
                    range: _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .EditorSelection */ .OF.cursor(pos + token.length) };
        }
        return { range: dont = range };
    });
    return dont ? null : state.update(changes, {
        scrollIntoView: true,
        userEvent: "input.type"
    });
}
function nodeStart(state, pos) {
    let tree = (0,_codemirror_language__WEBPACK_IMPORTED_MODULE_0__/* .syntaxTree */ .mv)(state).resolveInner(pos + 1);
    return tree.parent && tree.from == pos;
}
function probablyInString(state, pos, quoteToken, prefixes) {
    let node = (0,_codemirror_language__WEBPACK_IMPORTED_MODULE_0__/* .syntaxTree */ .mv)(state).resolveInner(pos, -1);
    let maxPrefix = prefixes.reduce((m, p) => Math.max(m, p.length), 0);
    for (let i = 0; i < 5; i++) {
        let start = state.sliceDoc(node.from, Math.min(node.to, node.from + quoteToken.length + maxPrefix));
        let quotePos = start.indexOf(quoteToken);
        if (!quotePos || quotePos > -1 && prefixes.indexOf(start.slice(0, quotePos)) > -1) {
            let first = node.firstChild;
            while (first && first.from == node.from && first.to - first.from > quoteToken.length + quotePos) {
                if (state.sliceDoc(first.to - quoteToken.length, first.to) == quoteToken)
                    return false;
                first = first.firstChild;
            }
            return true;
        }
        let parent = node.to == pos && node.parent;
        if (!parent)
            break;
        node = parent;
    }
    return false;
}
function canStartStringAt(state, pos, prefixes) {
    let charCat = state.charCategorizer(pos);
    if (charCat(state.sliceDoc(pos - 1, pos)) != _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .CharCategory */ .Je.Word)
        return pos;
    for (let prefix of prefixes) {
        let start = pos - prefix.length;
        if (state.sliceDoc(start, pos) == prefix && charCat(state.sliceDoc(start - 1, start)) != _codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .CharCategory */ .Je.Word)
            return start;
    }
    return -1;
}

/**
Returns an extension that enables autocompletion.
*/
function autocompletion(config = {}) {
    return [
        commitCharacters,
        completionState,
        completionConfig.of(config),
        completionPlugin,
        completionKeymapExt,
        baseTheme
    ];
}
/**
Basic keybindings for autocompletion.

 - Ctrl-Space (and Alt-\` or Alt-i on macOS): [`startCompletion`](https://codemirror.net/6/docs/ref/#autocomplete.startCompletion)
 - Escape: [`closeCompletion`](https://codemirror.net/6/docs/ref/#autocomplete.closeCompletion)
 - ArrowDown: [`moveCompletionSelection`](https://codemirror.net/6/docs/ref/#autocomplete.moveCompletionSelection)`(true)`
 - ArrowUp: [`moveCompletionSelection`](https://codemirror.net/6/docs/ref/#autocomplete.moveCompletionSelection)`(false)`
 - PageDown: [`moveCompletionSelection`](https://codemirror.net/6/docs/ref/#autocomplete.moveCompletionSelection)`(true, "page")`
 - PageUp: [`moveCompletionSelection`](https://codemirror.net/6/docs/ref/#autocomplete.moveCompletionSelection)`(false, "page")`
 - Enter: [`acceptCompletion`](https://codemirror.net/6/docs/ref/#autocomplete.acceptCompletion)
*/
const completionKeymap = [
    { key: "Ctrl-Space", run: startCompletion },
    { mac: "Alt-`", run: startCompletion },
    { mac: "Alt-i", run: startCompletion },
    { key: "Escape", run: closeCompletion },
    { key: "ArrowDown", run: /*@__PURE__*/moveCompletionSelection(true) },
    { key: "ArrowUp", run: /*@__PURE__*/moveCompletionSelection(false) },
    { key: "PageDown", run: /*@__PURE__*/moveCompletionSelection(true, "page") },
    { key: "PageUp", run: /*@__PURE__*/moveCompletionSelection(false, "page") },
    { key: "Enter", run: acceptCompletion }
];
const completionKeymapExt = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_1__/* .Prec */ .Nb.highest(/*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_2__/* .keymap */ .w4.computeN([completionConfig], state => state.facet(completionConfig).defaultKeymap ? [completionKeymap] : []));
/**
Get the current completion status. When completions are available,
this will return `"active"`. When completions are pending (in the
process of being queried), this returns `"pending"`. Otherwise, it
returns `null`.
*/
function completionStatus(state) {
    let cState = state.field(completionState, false);
    return cState && cState.active.some(a => a.isPending) ? "pending"
        : cState && cState.active.some(a => a.state != 0 /* State.Inactive */) ? "active" : null;
}
const completionArrayCache = /*@__PURE__*/new WeakMap;
/**
Returns the available completions as an array.
*/
function currentCompletions(state) {
    var _a;
    let open = (_a = state.field(completionState, false)) === null || _a === void 0 ? void 0 : _a.open;
    if (!open || open.disabled)
        return [];
    let completions = completionArrayCache.get(open.options);
    if (!completions)
        completionArrayCache.set(open.options, completions = open.options.map(o => o.completion));
    return completions;
}
/**
Return the currently selected completion, if any.
*/
function selectedCompletion(state) {
    var _a;
    let open = (_a = state.field(completionState, false)) === null || _a === void 0 ? void 0 : _a.open;
    return open && !open.disabled && open.selected >= 0 ? open.options[open.selected].completion : null;
}
/**
Returns the currently selected position in the active completion
list, or null if no completions are active.
*/
function selectedCompletionIndex(state) {
    var _a;
    let open = (_a = state.field(completionState, false)) === null || _a === void 0 ? void 0 : _a.open;
    return open && !open.disabled && open.selected >= 0 ? open.selected : null;
}
/**
Create an effect that can be attached to a transaction to change
the currently selected completion.
*/
function setSelectedCompletion(index) {
    return setSelectedEffect.of(index);
}




/***/ }),

/***/ 93756:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Eo: () => (/* binding */ searchKeymap),
/* harmony export */   Hg: () => (/* binding */ findNext),
/* harmony export */   gN: () => (/* binding */ highlightSelectionMatches),
/* harmony export */   pi: () => (/* binding */ openSearchPanel),
/* harmony export */   ve: () => (/* binding */ findPrevious)
/* harmony export */ });
/* unused harmony exports RegExpCursor, SearchCursor, SearchQuery, closeSearchPanel, getSearchQuery, gotoLine, replaceAll, replaceNext, search, searchPanelOpen, selectMatches, selectNextOccurrence, selectSelectionMatches, setSearchQuery */
/* harmony import */ var _codemirror_view__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(87748);
/* harmony import */ var _codemirror_state__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(64614);
/* harmony import */ var crelt__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(59984);




const basicNormalize = typeof String.prototype.normalize == "function"
    ? x => x.normalize("NFKD") : x => x;
/**
A search cursor provides an iterator over text matches in a
document.
*/
class SearchCursor {
    /**
    Create a text cursor. The query is the search string, `from` to
    `to` provides the region to search.
    
    When `normalize` is given, it will be called, on both the query
    string and the content it is matched against, before comparing.
    You can, for example, create a case-insensitive search by
    passing `s => s.toLowerCase()`.
    
    Text is always normalized with
    [`.normalize("NFKD")`](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/String/normalize)
    (when supported).
    */
    constructor(text, query, from = 0, to = text.length, normalize, test) {
        this.test = test;
        /**
        The current match (only holds a meaningful value after
        [`next`](https://codemirror.net/6/docs/ref/#search.SearchCursor.next) has been called and when
        `done` is false).
        */
        this.value = { from: 0, to: 0 };
        /**
        Whether the end of the iterated region has been reached.
        */
        this.done = false;
        this.matches = [];
        this.buffer = "";
        this.bufferPos = 0;
        this.iter = text.iterRange(from, to);
        this.bufferStart = from;
        this.normalize = normalize ? x => normalize(basicNormalize(x)) : basicNormalize;
        this.query = this.normalize(query);
    }
    peek() {
        if (this.bufferPos == this.buffer.length) {
            this.bufferStart += this.buffer.length;
            this.iter.next();
            if (this.iter.done)
                return -1;
            this.bufferPos = 0;
            this.buffer = this.iter.value;
        }
        return (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .codePointAt */ .vS)(this.buffer, this.bufferPos);
    }
    /**
    Look for the next match. Updates the iterator's
    [`value`](https://codemirror.net/6/docs/ref/#search.SearchCursor.value) and
    [`done`](https://codemirror.net/6/docs/ref/#search.SearchCursor.done) properties. Should be called
    at least once before using the cursor.
    */
    next() {
        while (this.matches.length)
            this.matches.pop();
        return this.nextOverlapping();
    }
    /**
    The `next` method will ignore matches that partially overlap a
    previous match. This method behaves like `next`, but includes
    such matches.
    */
    nextOverlapping() {
        for (;;) {
            let next = this.peek();
            if (next < 0) {
                this.done = true;
                return this;
            }
            let str = (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .fromCodePoint */ .MK)(next), start = this.bufferStart + this.bufferPos;
            this.bufferPos += (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .codePointSize */ .Fh)(next);
            let norm = this.normalize(str);
            if (norm.length)
                for (let i = 0, pos = start;; i++) {
                    let code = norm.charCodeAt(i);
                    let match = this.match(code, pos, this.bufferPos + this.bufferStart);
                    if (i == norm.length - 1) {
                        if (match) {
                            this.value = match;
                            return this;
                        }
                        break;
                    }
                    if (pos == start && i < str.length && str.charCodeAt(i) == code)
                        pos++;
                }
        }
    }
    match(code, pos, end) {
        let match = null;
        for (let i = 0; i < this.matches.length; i += 2) {
            let index = this.matches[i], keep = false;
            if (this.query.charCodeAt(index) == code) {
                if (index == this.query.length - 1) {
                    match = { from: this.matches[i + 1], to: end };
                }
                else {
                    this.matches[i]++;
                    keep = true;
                }
            }
            if (!keep) {
                this.matches.splice(i, 2);
                i -= 2;
            }
        }
        if (this.query.charCodeAt(0) == code) {
            if (this.query.length == 1)
                match = { from: pos, to: end };
            else
                this.matches.push(1, pos);
        }
        if (match && this.test && !this.test(match.from, match.to, this.buffer, this.bufferStart))
            match = null;
        return match;
    }
}
if (typeof Symbol != "undefined")
    SearchCursor.prototype[Symbol.iterator] = function () { return this; };

const empty = { from: -1, to: -1, match: /*@__PURE__*//.*/.exec("") };
const baseFlags = "gm" + (/x/.unicode == null ? "" : "u");
/**
This class is similar to [`SearchCursor`](https://codemirror.net/6/docs/ref/#search.SearchCursor)
but searches for a regular expression pattern instead of a plain
string.
*/
class RegExpCursor {
    /**
    Create a cursor that will search the given range in the given
    document. `query` should be the raw pattern (as you'd pass it to
    `new RegExp`).
    */
    constructor(text, query, options, from = 0, to = text.length) {
        this.text = text;
        this.to = to;
        this.curLine = "";
        /**
        Set to `true` when the cursor has reached the end of the search
        range.
        */
        this.done = false;
        /**
        Will contain an object with the extent of the match and the
        match object when [`next`](https://codemirror.net/6/docs/ref/#search.RegExpCursor.next)
        sucessfully finds a match.
        */
        this.value = empty;
        if (/\\[sWDnr]|\n|\r|\[\^/.test(query))
            return new MultilineRegExpCursor(text, query, options, from, to);
        this.re = new RegExp(query, baseFlags + ((options === null || options === void 0 ? void 0 : options.ignoreCase) ? "i" : ""));
        this.test = options === null || options === void 0 ? void 0 : options.test;
        this.iter = text.iter();
        let startLine = text.lineAt(from);
        this.curLineStart = startLine.from;
        this.matchPos = toCharEnd(text, from);
        this.getLine(this.curLineStart);
    }
    getLine(skip) {
        this.iter.next(skip);
        if (this.iter.lineBreak) {
            this.curLine = "";
        }
        else {
            this.curLine = this.iter.value;
            if (this.curLineStart + this.curLine.length > this.to)
                this.curLine = this.curLine.slice(0, this.to - this.curLineStart);
            this.iter.next();
        }
    }
    nextLine() {
        this.curLineStart = this.curLineStart + this.curLine.length + 1;
        if (this.curLineStart > this.to)
            this.curLine = "";
        else
            this.getLine(0);
    }
    /**
    Move to the next match, if there is one.
    */
    next() {
        for (let off = this.matchPos - this.curLineStart;;) {
            this.re.lastIndex = off;
            let match = this.matchPos <= this.to && this.re.exec(this.curLine);
            if (match) {
                let from = this.curLineStart + match.index, to = from + match[0].length;
                this.matchPos = toCharEnd(this.text, to + (from == to ? 1 : 0));
                if (from == this.curLineStart + this.curLine.length)
                    this.nextLine();
                if ((from < to || from > this.value.to) && (!this.test || this.test(from, to, match))) {
                    this.value = { from, to, match };
                    return this;
                }
                off = this.matchPos - this.curLineStart;
            }
            else if (this.curLineStart + this.curLine.length < this.to) {
                this.nextLine();
                off = 0;
            }
            else {
                this.done = true;
                return this;
            }
        }
    }
}
const flattened = /*@__PURE__*/new WeakMap();
// Reusable (partially) flattened document strings
class FlattenedDoc {
    constructor(from, text) {
        this.from = from;
        this.text = text;
    }
    get to() { return this.from + this.text.length; }
    static get(doc, from, to) {
        let cached = flattened.get(doc);
        if (!cached || cached.from >= to || cached.to <= from) {
            let flat = new FlattenedDoc(from, doc.sliceString(from, to));
            flattened.set(doc, flat);
            return flat;
        }
        if (cached.from == from && cached.to == to)
            return cached;
        let { text, from: cachedFrom } = cached;
        if (cachedFrom > from) {
            text = doc.sliceString(from, cachedFrom) + text;
            cachedFrom = from;
        }
        if (cached.to < to)
            text += doc.sliceString(cached.to, to);
        flattened.set(doc, new FlattenedDoc(cachedFrom, text));
        return new FlattenedDoc(from, text.slice(from - cachedFrom, to - cachedFrom));
    }
}
class MultilineRegExpCursor {
    constructor(text, query, options, from, to) {
        this.text = text;
        this.to = to;
        this.done = false;
        this.value = empty;
        this.matchPos = toCharEnd(text, from);
        this.re = new RegExp(query, baseFlags + ((options === null || options === void 0 ? void 0 : options.ignoreCase) ? "i" : ""));
        this.test = options === null || options === void 0 ? void 0 : options.test;
        this.flat = FlattenedDoc.get(text, from, this.chunkEnd(from + 5000 /* Chunk.Base */));
    }
    chunkEnd(pos) {
        return pos >= this.to ? this.to : this.text.lineAt(pos).to;
    }
    next() {
        for (;;) {
            let off = this.re.lastIndex = this.matchPos - this.flat.from;
            let match = this.re.exec(this.flat.text);
            // Skip empty matches directly after the last match
            if (match && !match[0] && match.index == off) {
                this.re.lastIndex = off + 1;
                match = this.re.exec(this.flat.text);
            }
            if (match) {
                let from = this.flat.from + match.index, to = from + match[0].length;
                // If a match goes almost to the end of a noncomplete chunk, try
                // again, since it'll likely be able to match more
                if ((this.flat.to >= this.to || match.index + match[0].length <= this.flat.text.length - 10) &&
                    (!this.test || this.test(from, to, match))) {
                    this.value = { from, to, match };
                    this.matchPos = toCharEnd(this.text, to + (from == to ? 1 : 0));
                    return this;
                }
            }
            if (this.flat.to == this.to) {
                this.done = true;
                return this;
            }
            // Grow the flattened doc
            this.flat = FlattenedDoc.get(this.text, this.flat.from, this.chunkEnd(this.flat.from + this.flat.text.length * 2));
        }
    }
}
if (typeof Symbol != "undefined") {
    RegExpCursor.prototype[Symbol.iterator] = MultilineRegExpCursor.prototype[Symbol.iterator] =
        function () { return this; };
}
function validRegExp(source) {
    try {
        new RegExp(source, baseFlags);
        return true;
    }
    catch (_a) {
        return false;
    }
}
function toCharEnd(text, pos) {
    if (pos >= text.length)
        return pos;
    let line = text.lineAt(pos), next;
    while (pos < line.to && (next = line.text.charCodeAt(pos - line.from)) >= 0xDC00 && next < 0xE000)
        pos++;
    return pos;
}

/**
Command that shows a dialog asking the user for a line number, and
when a valid position is provided, moves the cursor to that line.

Supports line numbers, relative line offsets prefixed with `+` or
`-`, document percentages suffixed with `%`, and an optional
column position by adding `:` and a second number after the line
number.
*/
const gotoLine = view => {
    let { state } = view;
    let line = String(state.doc.lineAt(view.state.selection.main.head).number);
    let { close, result } = (0,_codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .showDialog */ .ui)(view, {
        label: state.phrase("Go to line"),
        input: { type: "text", name: "line", value: line },
        focus: true,
        submitLabel: state.phrase("go"),
    });
    result.then(form => {
        let match = form && /^([+-])?(\d+)?(:\d+)?(%)?$/.exec(form.elements["line"].value);
        if (!match) {
            view.dispatch({ effects: close });
            return;
        }
        let startLine = state.doc.lineAt(state.selection.main.head);
        let [, sign, ln, cl, percent] = match;
        let col = cl ? +cl.slice(1) : 0;
        let line = ln ? +ln : startLine.number;
        if (ln && percent) {
            let pc = line / 100;
            if (sign)
                pc = pc * (sign == "-" ? -1 : 1) + (startLine.number / state.doc.lines);
            line = Math.round(state.doc.lines * pc);
        }
        else if (ln && sign) {
            line = line * (sign == "-" ? -1 : 1) + startLine.number;
        }
        let docLine = state.doc.line(Math.max(1, Math.min(state.doc.lines, line)));
        let selection = _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .EditorSelection */ .OF.cursor(docLine.from + Math.max(0, Math.min(col, docLine.length)));
        view.dispatch({
            effects: [close, _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .EditorView */ .Lz.scrollIntoView(selection.from, { y: 'center' })],
            selection,
        });
    });
    return true;
};

const defaultHighlightOptions = {
    highlightWordAroundCursor: false,
    minSelectionLength: 1,
    maxMatches: 100,
    wholeWords: false
};
const highlightConfig = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .Facet */ .sj.define({
    combine(options) {
        return (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .combineConfig */ .QR)(options, defaultHighlightOptions, {
            highlightWordAroundCursor: (a, b) => a || b,
            minSelectionLength: Math.min,
            maxMatches: Math.min
        });
    }
});
/**
This extension highlights text that matches the selection. It uses
the `"cm-selectionMatch"` class for the highlighting. When
`highlightWordAroundCursor` is enabled, the word at the cursor
itself will be highlighted with `"cm-selectionMatch-main"`.
*/
function highlightSelectionMatches(options) {
    let ext = [defaultTheme, matchHighlighter];
    if (options)
        ext.push(highlightConfig.of(options));
    return ext;
}
const matchDeco = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .Decoration */ .NZ.mark({ class: "cm-selectionMatch" });
const mainMatchDeco = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .Decoration */ .NZ.mark({ class: "cm-selectionMatch cm-selectionMatch-main" });
// Whether the characters directly outside the given positions are non-word characters
function insideWordBoundaries(check, state, from, to) {
    return (from == 0 || check(state.sliceDoc(from - 1, from)) != _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .CharCategory */ .Je.Word) &&
        (to == state.doc.length || check(state.sliceDoc(to, to + 1)) != _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .CharCategory */ .Je.Word);
}
// Whether the characters directly at the given positions are word characters
function insideWord(check, state, from, to) {
    return check(state.sliceDoc(from, from + 1)) == _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .CharCategory */ .Je.Word
        && check(state.sliceDoc(to - 1, to)) == _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .CharCategory */ .Je.Word;
}
const matchHighlighter = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .ViewPlugin */ .Z9.fromClass(class {
    constructor(view) {
        this.decorations = this.getDeco(view);
    }
    update(update) {
        if (update.selectionSet || update.docChanged || update.viewportChanged)
            this.decorations = this.getDeco(update.view);
    }
    getDeco(view) {
        let conf = view.state.facet(highlightConfig);
        let { state } = view, sel = state.selection;
        if (sel.ranges.length > 1)
            return _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .Decoration */ .NZ.none;
        let range = sel.main, query, check = null;
        if (range.empty) {
            if (!conf.highlightWordAroundCursor)
                return _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .Decoration */ .NZ.none;
            let word = state.wordAt(range.head);
            if (!word)
                return _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .Decoration */ .NZ.none;
            check = state.charCategorizer(range.head);
            query = state.sliceDoc(word.from, word.to);
        }
        else {
            let len = range.to - range.from;
            if (len < conf.minSelectionLength || len > 200)
                return _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .Decoration */ .NZ.none;
            if (conf.wholeWords) {
                query = state.sliceDoc(range.from, range.to); // TODO: allow and include leading/trailing space?
                check = state.charCategorizer(range.head);
                if (!(insideWordBoundaries(check, state, range.from, range.to) &&
                    insideWord(check, state, range.from, range.to)))
                    return _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .Decoration */ .NZ.none;
            }
            else {
                query = state.sliceDoc(range.from, range.to);
                if (!query)
                    return _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .Decoration */ .NZ.none;
            }
        }
        let deco = [];
        for (let part of view.visibleRanges) {
            let cursor = new SearchCursor(state.doc, query, part.from, part.to);
            while (!cursor.next().done) {
                let { from, to } = cursor.value;
                if (!check || insideWordBoundaries(check, state, from, to)) {
                    if (range.empty && from <= range.from && to >= range.to)
                        deco.push(mainMatchDeco.range(from, to));
                    else if (from >= range.to || to <= range.from)
                        deco.push(matchDeco.range(from, to));
                    if (deco.length > conf.maxMatches)
                        return _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .Decoration */ .NZ.none;
                }
            }
        }
        return _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .Decoration */ .NZ.set(deco);
    }
}, {
    decorations: v => v.decorations
});
const defaultTheme = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .EditorView */ .Lz.baseTheme({
    ".cm-selectionMatch": { backgroundColor: "#99ff7780" },
    ".cm-searchMatch .cm-selectionMatch": { backgroundColor: "transparent" }
});
// Select the words around the cursors.
const selectWord = ({ state, dispatch }) => {
    let { selection } = state;
    let newSel = _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .EditorSelection */ .OF.create(selection.ranges.map(range => state.wordAt(range.head) || _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .EditorSelection */ .OF.cursor(range.head)), selection.mainIndex);
    if (newSel.eq(selection))
        return false;
    dispatch(state.update({ selection: newSel }));
    return true;
};
// Find next occurrence of query relative to last cursor. Wrap around
// the document if there are no more matches.
function findNextOccurrence(state, query) {
    let { main, ranges } = state.selection;
    let word = state.wordAt(main.head), fullWord = word && word.from == main.from && word.to == main.to;
    for (let cycled = false, cursor = new SearchCursor(state.doc, query, ranges[ranges.length - 1].to);;) {
        cursor.next();
        if (cursor.done) {
            if (cycled)
                return null;
            cursor = new SearchCursor(state.doc, query, 0, Math.max(0, ranges[ranges.length - 1].from - 1));
            cycled = true;
        }
        else {
            if (cycled && ranges.some(r => r.from == cursor.value.from))
                continue;
            if (fullWord) {
                let word = state.wordAt(cursor.value.from);
                if (!word || word.from != cursor.value.from || word.to != cursor.value.to)
                    continue;
            }
            return cursor.value;
        }
    }
}
/**
Select next occurrence of the current selection. Expand selection
to the surrounding word when the selection is empty.
*/
const selectNextOccurrence = ({ state, dispatch }) => {
    let { ranges } = state.selection;
    if (ranges.some(sel => sel.from === sel.to))
        return selectWord({ state, dispatch });
    let searchedText = state.sliceDoc(ranges[0].from, ranges[0].to);
    if (state.selection.ranges.some(r => state.sliceDoc(r.from, r.to) != searchedText))
        return false;
    let range = findNextOccurrence(state, searchedText);
    if (!range)
        return false;
    dispatch(state.update({
        selection: state.selection.addRange(_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .EditorSelection */ .OF.range(range.from, range.to), false),
        effects: _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .EditorView */ .Lz.scrollIntoView(range.to)
    }));
    return true;
};

const searchConfigFacet = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .Facet */ .sj.define({
    combine(configs) {
        return (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .combineConfig */ .QR)(configs, {
            top: false,
            caseSensitive: false,
            literal: false,
            regexp: false,
            wholeWord: false,
            createPanel: view => new SearchPanel(view),
            scrollToMatch: range => _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .EditorView */ .Lz.scrollIntoView(range)
        });
    }
});
/**
Add search state to the editor configuration, and optionally
configure the search extension.
([`openSearchPanel`](https://codemirror.net/6/docs/ref/#search.openSearchPanel) will automatically
enable this if it isn't already on).
*/
function search(config) {
    return config ? [searchConfigFacet.of(config), searchExtensions] : searchExtensions;
}
/**
A search query. Part of the editor's search state.
*/
class SearchQuery {
    /**
    Create a query object.
    */
    constructor(config) {
        this.search = config.search;
        this.caseSensitive = !!config.caseSensitive;
        this.literal = !!config.literal;
        this.regexp = !!config.regexp;
        this.replace = config.replace || "";
        this.valid = !!this.search && (!this.regexp || validRegExp(this.search));
        this.unquoted = this.unquote(this.search);
        this.wholeWord = !!config.wholeWord;
        this.test = config.test;
    }
    /**
    @internal
    */
    unquote(text) {
        return this.literal ? text :
            text.replace(/\\([nrt\\])/g, (_, ch) => ch == "n" ? "\n" : ch == "r" ? "\r" : ch == "t" ? "\t" : "\\");
    }
    /**
    Compare this query to another query.
    */
    eq(other) {
        return this.search == other.search && this.replace == other.replace &&
            this.caseSensitive == other.caseSensitive && this.regexp == other.regexp &&
            this.wholeWord == other.wholeWord && this.test == other.test;
    }
    /**
    @internal
    */
    create() {
        return this.regexp ? new RegExpQuery(this) : new StringQuery(this);
    }
    /**
    Get a search cursor for this query, searching through the given
    range in the given state.
    */
    getCursor(state, from = 0, to) {
        let st = state.doc ? state : _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .EditorState */ .$t.create({ doc: state });
        if (to == null)
            to = st.doc.length;
        return this.regexp ? regexpCursor(this, st, from, to) : stringCursor(this, st, from, to);
    }
}
class QueryType {
    constructor(spec) {
        this.spec = spec;
    }
}
function wrapStringTest(test, state, inner) {
    return (from, to, buffer, bufferPos) => {
        if (inner && !inner(from, to, buffer, bufferPos))
            return false;
        let match = from >= bufferPos && to <= bufferPos + buffer.length
            ? buffer.slice(from - bufferPos, to - bufferPos)
            : state.doc.sliceString(from, to);
        return test(match, state, from, to);
    };
}
function stringCursor(spec, state, from, to) {
    let test;
    if (spec.wholeWord)
        test = stringWordTest(state.doc, state.charCategorizer(state.selection.main.head));
    if (spec.test)
        test = wrapStringTest(spec.test, state, test);
    return new SearchCursor(state.doc, spec.unquoted, from, to, spec.caseSensitive ? undefined : x => x.toLowerCase(), test);
}
function stringWordTest(doc, categorizer) {
    return (from, to, buf, bufPos) => {
        if (bufPos > from || bufPos + buf.length < to) {
            bufPos = Math.max(0, from - 2);
            buf = doc.sliceString(bufPos, Math.min(doc.length, to + 2));
        }
        return (categorizer(charBefore(buf, from - bufPos)) != _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .CharCategory */ .Je.Word ||
            categorizer(charAfter(buf, from - bufPos)) != _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .CharCategory */ .Je.Word) &&
            (categorizer(charAfter(buf, to - bufPos)) != _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .CharCategory */ .Je.Word ||
                categorizer(charBefore(buf, to - bufPos)) != _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .CharCategory */ .Je.Word);
    };
}
class StringQuery extends QueryType {
    constructor(spec) {
        super(spec);
    }
    nextMatch(state, curFrom, curTo) {
        let cursor = stringCursor(this.spec, state, curTo, state.doc.length).nextOverlapping();
        if (cursor.done) {
            let end = Math.min(state.doc.length, curFrom + this.spec.unquoted.length);
            cursor = stringCursor(this.spec, state, 0, end).nextOverlapping();
        }
        return cursor.done || cursor.value.from == curFrom && cursor.value.to == curTo ? null : cursor.value;
    }
    // Searching in reverse is, rather than implementing an inverted search
    // cursor, done by scanning chunk after chunk forward.
    prevMatchInRange(state, from, to) {
        for (let pos = to;;) {
            let start = Math.max(from, pos - 10000 /* FindPrev.ChunkSize */ - this.spec.unquoted.length);
            let cursor = stringCursor(this.spec, state, start, pos), range = null;
            while (!cursor.nextOverlapping().done)
                range = cursor.value;
            if (range)
                return range;
            if (start == from)
                return null;
            pos -= 10000 /* FindPrev.ChunkSize */;
        }
    }
    prevMatch(state, curFrom, curTo) {
        let found = this.prevMatchInRange(state, 0, curFrom);
        if (!found)
            found = this.prevMatchInRange(state, Math.max(0, curTo - this.spec.unquoted.length), state.doc.length);
        return found && (found.from != curFrom || found.to != curTo) ? found : null;
    }
    getReplacement(_result) { return this.spec.unquote(this.spec.replace); }
    matchAll(state, limit) {
        let cursor = stringCursor(this.spec, state, 0, state.doc.length), ranges = [];
        while (!cursor.next().done) {
            if (ranges.length >= limit)
                return null;
            ranges.push(cursor.value);
        }
        return ranges;
    }
    highlight(state, from, to, add) {
        let cursor = stringCursor(this.spec, state, Math.max(0, from - this.spec.unquoted.length), Math.min(to + this.spec.unquoted.length, state.doc.length));
        while (!cursor.next().done)
            add(cursor.value.from, cursor.value.to);
    }
}
function wrapRegexpTest(test, state, inner) {
    return (from, to, match) => {
        return (!inner || inner(from, to, match)) && test(match[0], state, from, to);
    };
}
function regexpCursor(spec, state, from, to) {
    let test;
    if (spec.wholeWord)
        test = regexpWordTest(state.charCategorizer(state.selection.main.head));
    if (spec.test)
        test = wrapRegexpTest(spec.test, state, test);
    return new RegExpCursor(state.doc, spec.search, { ignoreCase: !spec.caseSensitive, test }, from, to);
}
function charBefore(str, index) {
    return str.slice((0,_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .findClusterBreak */ .zK)(str, index, false), index);
}
function charAfter(str, index) {
    return str.slice(index, (0,_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .findClusterBreak */ .zK)(str, index));
}
function regexpWordTest(categorizer) {
    return (_from, _to, match) => !match[0].length ||
        (categorizer(charBefore(match.input, match.index)) != _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .CharCategory */ .Je.Word ||
            categorizer(charAfter(match.input, match.index)) != _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .CharCategory */ .Je.Word) &&
            (categorizer(charAfter(match.input, match.index + match[0].length)) != _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .CharCategory */ .Je.Word ||
                categorizer(charBefore(match.input, match.index + match[0].length)) != _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .CharCategory */ .Je.Word);
}
class RegExpQuery extends QueryType {
    nextMatch(state, curFrom, curTo) {
        let cursor = regexpCursor(this.spec, state, curTo, state.doc.length).next();
        if (cursor.done)
            cursor = regexpCursor(this.spec, state, 0, curFrom).next();
        return cursor.done ? null : cursor.value;
    }
    prevMatchInRange(state, from, to) {
        for (let size = 1;; size++) {
            let start = Math.max(from, to - size * 10000 /* FindPrev.ChunkSize */);
            let cursor = regexpCursor(this.spec, state, start, to), range = null;
            while (!cursor.next().done)
                range = cursor.value;
            if (range && (start == from || range.from > start + 10))
                return range;
            if (start == from)
                return null;
        }
    }
    prevMatch(state, curFrom, curTo) {
        return this.prevMatchInRange(state, 0, curFrom) ||
            this.prevMatchInRange(state, curTo, state.doc.length);
    }
    getReplacement(result) {
        return this.spec.unquote(this.spec.replace).replace(/\$([$&]|\d+)/g, (m, i) => {
            if (i == "&")
                return result.match[0];
            if (i == "$")
                return "$";
            for (let l = i.length; l > 0; l--) {
                let n = +i.slice(0, l);
                if (n > 0 && n < result.match.length)
                    return result.match[n] + i.slice(l);
            }
            return m;
        });
    }
    matchAll(state, limit) {
        let cursor = regexpCursor(this.spec, state, 0, state.doc.length), ranges = [];
        while (!cursor.next().done) {
            if (ranges.length >= limit)
                return null;
            ranges.push(cursor.value);
        }
        return ranges;
    }
    highlight(state, from, to, add) {
        let cursor = regexpCursor(this.spec, state, Math.max(0, from - 250 /* RegExp.HighlightMargin */), Math.min(to + 250 /* RegExp.HighlightMargin */, state.doc.length));
        while (!cursor.next().done)
            add(cursor.value.from, cursor.value.to);
    }
}
/**
A state effect that updates the current search query. Note that
this only has an effect if the search state has been initialized
(by including [`search`](https://codemirror.net/6/docs/ref/#search.search) in your configuration or
by running [`openSearchPanel`](https://codemirror.net/6/docs/ref/#search.openSearchPanel) at least
once).
*/
const setSearchQuery = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .StateEffect */ .Pe.define();
const togglePanel = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .StateEffect */ .Pe.define();
const searchState = /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .StateField */ .sU.define({
    create(state) {
        return new SearchState(defaultQuery(state).create(), null);
    },
    update(value, tr) {
        for (let effect of tr.effects) {
            if (effect.is(setSearchQuery))
                value = new SearchState(effect.value.create(), value.panel);
            else if (effect.is(togglePanel))
                value = new SearchState(value.query, effect.value ? createSearchPanel : null);
        }
        return value;
    },
    provide: f => _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .showPanel */ .S7.from(f, val => val.panel)
});
/**
Get the current search query from an editor state.
*/
function getSearchQuery(state) {
    let curState = state.field(searchState, false);
    return curState ? curState.query.spec : defaultQuery(state);
}
/**
Query whether the search panel is open in the given editor state.
*/
function searchPanelOpen(state) {
    var _a;
    return ((_a = state.field(searchState, false)) === null || _a === void 0 ? void 0 : _a.panel) != null;
}
class SearchState {
    constructor(query, panel) {
        this.query = query;
        this.panel = panel;
    }
}
const matchMark = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .Decoration */ .NZ.mark({ class: "cm-searchMatch" }), selectedMatchMark = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .Decoration */ .NZ.mark({ class: "cm-searchMatch cm-searchMatch-selected" });
const searchHighlighter = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .ViewPlugin */ .Z9.fromClass(class {
    constructor(view) {
        this.view = view;
        this.decorations = this.highlight(view.state.field(searchState));
    }
    update(update) {
        let state = update.state.field(searchState);
        if (state != update.startState.field(searchState) || update.docChanged || update.selectionSet || update.viewportChanged)
            this.decorations = this.highlight(state);
    }
    highlight({ query, panel }) {
        if (!panel || !query.spec.valid)
            return _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .Decoration */ .NZ.none;
        let { view } = this;
        let builder = new _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .RangeSetBuilder */ .vB();
        for (let i = 0, ranges = view.visibleRanges, l = ranges.length; i < l; i++) {
            let { from, to } = ranges[i];
            while (i < l - 1 && to > ranges[i + 1].from - 2 * 250 /* RegExp.HighlightMargin */)
                to = ranges[++i].to;
            query.highlight(view.state, from, to, (from, to) => {
                let selected = view.state.selection.ranges.some(r => r.from == from && r.to == to);
                builder.add(from, to, selected ? selectedMatchMark : matchMark);
            });
        }
        return builder.finish();
    }
}, {
    decorations: v => v.decorations
});
function searchCommand(f) {
    return view => {
        let state = view.state.field(searchState, false);
        return state && state.query.spec.valid ? f(view, state) : openSearchPanel(view);
    };
}
/**
Open the search panel if it isn't already open, and move the
selection to the first match after the current main selection.
Will wrap around to the start of the document when it reaches the
end.
*/
const findNext = /*@__PURE__*/searchCommand((view, { query }) => {
    let { to } = view.state.selection.main;
    let next = query.nextMatch(view.state, to, to);
    if (!next)
        return false;
    let selection = _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .EditorSelection */ .OF.single(next.from, next.to);
    let config = view.state.facet(searchConfigFacet);
    view.dispatch({
        selection,
        effects: [announceMatch(view, next), config.scrollToMatch(selection.main, view)],
        userEvent: "select.search"
    });
    selectSearchInput(view);
    return true;
});
/**
Move the selection to the previous instance of the search query,
before the current main selection. Will wrap past the start
of the document to start searching at the end again.
*/
const findPrevious = /*@__PURE__*/searchCommand((view, { query }) => {
    let { state } = view, { from } = state.selection.main;
    let prev = query.prevMatch(state, from, from);
    if (!prev)
        return false;
    let selection = _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .EditorSelection */ .OF.single(prev.from, prev.to);
    let config = view.state.facet(searchConfigFacet);
    view.dispatch({
        selection,
        effects: [announceMatch(view, prev), config.scrollToMatch(selection.main, view)],
        userEvent: "select.search"
    });
    selectSearchInput(view);
    return true;
});
/**
Select all instances of the search query.
*/
const selectMatches = /*@__PURE__*/searchCommand((view, { query }) => {
    let ranges = query.matchAll(view.state, 1000);
    if (!ranges || !ranges.length)
        return false;
    view.dispatch({
        selection: _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .EditorSelection */ .OF.create(ranges.map(r => _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .EditorSelection */ .OF.range(r.from, r.to))),
        userEvent: "select.search.matches"
    });
    return true;
});
/**
Select all instances of the currently selected text.
*/
const selectSelectionMatches = ({ state, dispatch }) => {
    let sel = state.selection;
    if (sel.ranges.length > 1 || sel.main.empty)
        return false;
    let { from, to } = sel.main;
    let ranges = [], main = 0;
    for (let cur = new SearchCursor(state.doc, state.sliceDoc(from, to)); !cur.next().done;) {
        if (ranges.length > 1000)
            return false;
        if (cur.value.from == from)
            main = ranges.length;
        ranges.push(_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .EditorSelection */ .OF.range(cur.value.from, cur.value.to));
    }
    dispatch(state.update({
        selection: _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .EditorSelection */ .OF.create(ranges, main),
        userEvent: "select.search.matches"
    }));
    return true;
};
/**
Replace the current match of the search query.
*/
const replaceNext = /*@__PURE__*/searchCommand((view, { query }) => {
    let { state } = view, { from, to } = state.selection.main;
    if (state.readOnly)
        return false;
    let match = query.nextMatch(state, from, from);
    if (!match)
        return false;
    let next = match;
    let changes = [], selection, replacement;
    let effects = [];
    if (next.from == from && next.to == to) {
        replacement = state.toText(query.getReplacement(next));
        changes.push({ from: next.from, to: next.to, insert: replacement });
        next = query.nextMatch(state, next.from, next.to);
        effects.push(_codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .EditorView */ .Lz.announce.of(state.phrase("replaced match on line $", state.doc.lineAt(from).number) + "."));
    }
    let changeSet = view.state.changes(changes);
    if (next) {
        selection = _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .EditorSelection */ .OF.single(next.from, next.to).map(changeSet);
        effects.push(announceMatch(view, next));
        effects.push(state.facet(searchConfigFacet).scrollToMatch(selection.main, view));
    }
    view.dispatch({
        changes: changeSet,
        selection,
        effects,
        userEvent: "input.replace"
    });
    return true;
});
/**
Replace all instances of the search query with the given
replacement.
*/
const replaceAll = /*@__PURE__*/searchCommand((view, { query }) => {
    if (view.state.readOnly)
        return false;
    let changes = query.matchAll(view.state, 1e9).map(match => {
        let { from, to } = match;
        return { from, to, insert: query.getReplacement(match) };
    });
    if (!changes.length)
        return false;
    let announceText = view.state.phrase("replaced $ matches", changes.length) + ".";
    view.dispatch({
        changes,
        effects: _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .EditorView */ .Lz.announce.of(announceText),
        userEvent: "input.replace.all"
    });
    return true;
});
function createSearchPanel(view) {
    return view.state.facet(searchConfigFacet).createPanel(view);
}
function defaultQuery(state, fallback) {
    var _a, _b, _c, _d, _e;
    let sel = state.selection.main;
    let selText = sel.empty || sel.to > sel.from + 100 ? "" : state.sliceDoc(sel.from, sel.to);
    if (fallback && !selText)
        return fallback;
    let config = state.facet(searchConfigFacet);
    return new SearchQuery({
        search: ((_a = fallback === null || fallback === void 0 ? void 0 : fallback.literal) !== null && _a !== void 0 ? _a : config.literal) ? selText : selText.replace(/\n/g, "\\n"),
        caseSensitive: (_b = fallback === null || fallback === void 0 ? void 0 : fallback.caseSensitive) !== null && _b !== void 0 ? _b : config.caseSensitive,
        literal: (_c = fallback === null || fallback === void 0 ? void 0 : fallback.literal) !== null && _c !== void 0 ? _c : config.literal,
        regexp: (_d = fallback === null || fallback === void 0 ? void 0 : fallback.regexp) !== null && _d !== void 0 ? _d : config.regexp,
        wholeWord: (_e = fallback === null || fallback === void 0 ? void 0 : fallback.wholeWord) !== null && _e !== void 0 ? _e : config.wholeWord
    });
}
function getSearchInput(view) {
    let panel = (0,_codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .getPanel */ .ld)(view, createSearchPanel);
    return panel && panel.dom.querySelector("[main-field]");
}
function selectSearchInput(view) {
    let input = getSearchInput(view);
    if (input && input == view.root.activeElement)
        input.select();
}
/**
Make sure the search panel is open and focused.
*/
const openSearchPanel = view => {
    let state = view.state.field(searchState, false);
    if (state && state.panel) {
        let searchInput = getSearchInput(view);
        if (searchInput && searchInput != view.root.activeElement) {
            let query = defaultQuery(view.state, state.query.spec);
            if (query.valid)
                view.dispatch({ effects: setSearchQuery.of(query) });
            searchInput.focus();
            searchInput.select();
        }
    }
    else {
        view.dispatch({ effects: [
                togglePanel.of(true),
                state ? setSearchQuery.of(defaultQuery(view.state, state.query.spec)) : _codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .StateEffect */ .Pe.appendConfig.of(searchExtensions)
            ] });
    }
    return true;
};
/**
Close the search panel.
*/
const closeSearchPanel = view => {
    let state = view.state.field(searchState, false);
    if (!state || !state.panel)
        return false;
    let panel = (0,_codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .getPanel */ .ld)(view, createSearchPanel);
    if (panel && panel.dom.contains(view.root.activeElement))
        view.focus();
    view.dispatch({ effects: togglePanel.of(false) });
    return true;
};
/**
Default search-related key bindings.

 - Mod-f: [`openSearchPanel`](https://codemirror.net/6/docs/ref/#search.openSearchPanel)
 - F3, Mod-g: [`findNext`](https://codemirror.net/6/docs/ref/#search.findNext)
 - Shift-F3, Shift-Mod-g: [`findPrevious`](https://codemirror.net/6/docs/ref/#search.findPrevious)
 - Mod-Alt-g: [`gotoLine`](https://codemirror.net/6/docs/ref/#search.gotoLine)
 - Mod-d: [`selectNextOccurrence`](https://codemirror.net/6/docs/ref/#search.selectNextOccurrence)
*/
const searchKeymap = [
    { key: "Mod-f", run: openSearchPanel, scope: "editor search-panel" },
    { key: "F3", run: findNext, shift: findPrevious, scope: "editor search-panel", preventDefault: true },
    { key: "Mod-g", run: findNext, shift: findPrevious, scope: "editor search-panel", preventDefault: true },
    { key: "Escape", run: closeSearchPanel, scope: "editor search-panel" },
    { key: "Mod-Shift-l", run: selectSelectionMatches },
    { key: "Mod-Alt-g", run: gotoLine },
    { key: "Mod-d", run: selectNextOccurrence, preventDefault: true },
];
class SearchPanel {
    constructor(view) {
        this.view = view;
        let query = this.query = view.state.field(searchState).query.spec;
        this.commit = this.commit.bind(this);
        this.searchField = (0,crelt__WEBPACK_IMPORTED_MODULE_2__/* ["default"] */ .A)("input", {
            value: query.search,
            placeholder: phrase(view, "Find"),
            "aria-label": phrase(view, "Find"),
            class: "cm-textfield",
            name: "search",
            form: "",
            "main-field": "true",
            onchange: this.commit,
            onkeyup: this.commit
        });
        this.replaceField = (0,crelt__WEBPACK_IMPORTED_MODULE_2__/* ["default"] */ .A)("input", {
            value: query.replace,
            placeholder: phrase(view, "Replace"),
            "aria-label": phrase(view, "Replace"),
            class: "cm-textfield",
            name: "replace",
            form: "",
            onchange: this.commit,
            onkeyup: this.commit
        });
        this.caseField = (0,crelt__WEBPACK_IMPORTED_MODULE_2__/* ["default"] */ .A)("input", {
            type: "checkbox",
            name: "case",
            form: "",
            checked: query.caseSensitive,
            onchange: this.commit
        });
        this.reField = (0,crelt__WEBPACK_IMPORTED_MODULE_2__/* ["default"] */ .A)("input", {
            type: "checkbox",
            name: "re",
            form: "",
            checked: query.regexp,
            onchange: this.commit
        });
        this.wordField = (0,crelt__WEBPACK_IMPORTED_MODULE_2__/* ["default"] */ .A)("input", {
            type: "checkbox",
            name: "word",
            form: "",
            checked: query.wholeWord,
            onchange: this.commit
        });
        function button(name, onclick, content) {
            return (0,crelt__WEBPACK_IMPORTED_MODULE_2__/* ["default"] */ .A)("button", { class: "cm-button", name, onclick, type: "button" }, content);
        }
        this.dom = (0,crelt__WEBPACK_IMPORTED_MODULE_2__/* ["default"] */ .A)("div", { onkeydown: (e) => this.keydown(e), class: "cm-search" }, [
            this.searchField,
            button("next", () => findNext(view), [phrase(view, "next")]),
            button("prev", () => findPrevious(view), [phrase(view, "previous")]),
            button("select", () => selectMatches(view), [phrase(view, "all")]),
            (0,crelt__WEBPACK_IMPORTED_MODULE_2__/* ["default"] */ .A)("label", null, [this.caseField, phrase(view, "match case")]),
            (0,crelt__WEBPACK_IMPORTED_MODULE_2__/* ["default"] */ .A)("label", null, [this.reField, phrase(view, "regexp")]),
            (0,crelt__WEBPACK_IMPORTED_MODULE_2__/* ["default"] */ .A)("label", null, [this.wordField, phrase(view, "by word")]),
            ...view.state.readOnly ? [] : [
                (0,crelt__WEBPACK_IMPORTED_MODULE_2__/* ["default"] */ .A)("br"),
                this.replaceField,
                button("replace", () => replaceNext(view), [phrase(view, "replace")]),
                button("replaceAll", () => replaceAll(view), [phrase(view, "replace all")])
            ],
            (0,crelt__WEBPACK_IMPORTED_MODULE_2__/* ["default"] */ .A)("button", {
                name: "close",
                onclick: () => closeSearchPanel(view),
                "aria-label": phrase(view, "close"),
                type: "button"
            }, ["×"])
        ]);
    }
    commit() {
        let query = new SearchQuery({
            search: this.searchField.value,
            caseSensitive: this.caseField.checked,
            regexp: this.reField.checked,
            wholeWord: this.wordField.checked,
            replace: this.replaceField.value,
        });
        if (!query.eq(this.query)) {
            this.query = query;
            this.view.dispatch({ effects: setSearchQuery.of(query) });
        }
    }
    keydown(e) {
        if ((0,_codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .runScopeHandlers */ .TS)(this.view, e, "search-panel")) {
            e.preventDefault();
        }
        else if (e.keyCode == 13 && e.target == this.searchField) {
            e.preventDefault();
            (e.shiftKey ? findPrevious : findNext)(this.view);
        }
        else if (e.keyCode == 13 && e.target == this.replaceField) {
            e.preventDefault();
            replaceNext(this.view);
        }
    }
    update(update) {
        for (let tr of update.transactions)
            for (let effect of tr.effects) {
                if (effect.is(setSearchQuery) && !effect.value.eq(this.query))
                    this.setQuery(effect.value);
            }
    }
    setQuery(query) {
        this.query = query;
        this.searchField.value = query.search;
        this.replaceField.value = query.replace;
        this.caseField.checked = query.caseSensitive;
        this.reField.checked = query.regexp;
        this.wordField.checked = query.wholeWord;
    }
    mount() {
        this.searchField.select();
    }
    get pos() { return 80; }
    get top() { return this.view.state.facet(searchConfigFacet).top; }
}
function phrase(view, phrase) { return view.state.phrase(phrase); }
const AnnounceMargin = 30;
const Break = /[\s\.,:;?!]/;
function announceMatch(view, { from, to }) {
    let line = view.state.doc.lineAt(from), lineEnd = view.state.doc.lineAt(to).to;
    let start = Math.max(line.from, from - AnnounceMargin), end = Math.min(lineEnd, to + AnnounceMargin);
    let text = view.state.sliceDoc(start, end);
    if (start != line.from) {
        for (let i = 0; i < AnnounceMargin; i++)
            if (!Break.test(text[i + 1]) && Break.test(text[i])) {
                text = text.slice(i);
                break;
            }
    }
    if (end != lineEnd) {
        for (let i = text.length - 1; i > text.length - AnnounceMargin; i--)
            if (!Break.test(text[i - 1]) && Break.test(text[i])) {
                text = text.slice(0, i);
                break;
            }
    }
    return _codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .EditorView */ .Lz.announce.of(`${view.state.phrase("current match")}. ${text} ${view.state.phrase("on line")} ${line.number}.`);
}
const baseTheme = /*@__PURE__*/_codemirror_view__WEBPACK_IMPORTED_MODULE_1__/* .EditorView */ .Lz.baseTheme({
    ".cm-panel.cm-search": {
        padding: "2px 6px 4px",
        position: "relative",
        "& [name=close]": {
            position: "absolute",
            top: "0",
            right: "4px",
            backgroundColor: "inherit",
            border: "none",
            font: "inherit",
            padding: 0,
            margin: 0
        },
        "& input, & button, & label": {
            margin: ".2em .6em .2em 0"
        },
        "& input[type=checkbox]": {
            marginRight: ".2em"
        },
        "& label": {
            fontSize: "80%",
            whiteSpace: "pre"
        }
    },
    "&light .cm-searchMatch": { backgroundColor: "#ffff0054" },
    "&dark .cm-searchMatch": { backgroundColor: "#00ffff8a" },
    "&light .cm-searchMatch-selected": { backgroundColor: "#ff6a0054" },
    "&dark .cm-searchMatch-selected": { backgroundColor: "#ff00ff8a" }
});
const searchExtensions = [
    searchState,
    /*@__PURE__*/_codemirror_state__WEBPACK_IMPORTED_MODULE_0__/* .Prec */ .Nb.low(searchHighlighter),
    baseTheme
];




/***/ })

};
;