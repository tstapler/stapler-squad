"use strict";
exports.id = 3370;
exports.ids = [3370];
exports.modules = {

/***/ 10421:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  Ol: () => (/* binding */ FileContentViewer),
  od: () => (/* binding */ detectLanguage)
});

// UNUSED EXPORTS: DOUBLE_G_WINDOW_MS, isDoubleGPress

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ./src/lib/hooks/useFileService.ts
var useFileService = __webpack_require__(37353);
// EXTERNAL MODULE: ./src/components/sessions/files/LargeFileStates.tsx + 1 modules
var LargeFileStates = __webpack_require__(42467);
// EXTERNAL MODULE: ./src/lib/files/windowedFile.ts
var windowedFile = __webpack_require__(80335);
// EXTERNAL MODULE: ./src/lib/files/useWindowedState.ts
var useWindowedState = __webpack_require__(61991);
// EXTERNAL MODULE: ./src/lib/files/analytics/events.ts
var events = __webpack_require__(80852);
// EXTERNAL MODULE: ./src/lib/hooks/useFileTransport.ts + 5 modules
var useFileTransport = __webpack_require__(55500);
;// ./src/lib/files/degradeProfile.ts
/** What the editor may switch off so a large file stays responsive on a phone (ux.md U-MOB-5). */ /** Thresholds are constants so the Task 0.1.5b phone sitting can retune them. UTF-16 units, close enough to bytes for source. */ const DEGRADE_MIN_CHARS = 512 * 1024;
const LONG_LINE_CHARS = 4000;
const DEGRADE_NOTICE = "Large file: syntax highlighting, bracket matching and folding are off to keep scrolling smooth.";
const FULL_PROFILE = {
    degraded: false,
    syntaxHighlight: true,
    bracketMatching: true,
    activeLine: true,
    foldGutter: true,
    wrap: true,
    notice: null
};
function longestLineLength(doc) {
    let longest = 0;
    let start = 0;
    for(let i = 0; i <= doc.length; i++){
        if (i === doc.length || doc.charCodeAt(i) === 10) {
            const end = i > start && doc.charCodeAt(i - 1) === 13 ? i - 1 : i;
            if (end - start > longest) longest = end - start;
            start = i + 1;
        }
    }
    return longest;
}
/**
 * Desktop and small files keep everything; only a coarse pointer degrades. A windowed file (over 1 MiB, served by
 * range reads) is never degraded whole: `doc` is only the loaded text, which still decides wrapping.
 */ function degradeProfile({ doc, pointerCoarse, windowed = false }) {
    if (!pointerCoarse) return FULL_PROFILE;
    const wrap = longestLineLength(doc) < LONG_LINE_CHARS;
    if (windowed || doc.length < DEGRADE_MIN_CHARS) return wrap ? FULL_PROFILE : {
        ...FULL_PROFILE,
        wrap
    };
    return {
        degraded: true,
        syntaxHighlight: false,
        bracketMatching: false,
        activeLine: false,
        foldGutter: false,
        wrap,
        notice: DEGRADE_NOTICE
    };
}

// EXTERNAL MODULE: ./src/lib/files/fontSize.ts
var fontSize = __webpack_require__(96952);
// EXTERNAL MODULE: ./src/lib/hooks/useCoarsePointer.ts
var useCoarsePointer = __webpack_require__(51410);
// EXTERNAL MODULE: ./src/components/sessions/CodeEditor.css.ts
var CodeEditor_css = __webpack_require__(76312);
;// ./src/components/sessions/CodeEditor.tsx
/* __next_internal_client_entry_do_not_use__ CodeEditor auto */ 





/** `doc` is the text the buffer is created from; the large-file profile is decided per buffer, not per prop. */ function modeConfig(p, device, doc) {
    const mode = p.mode ?? "Read";
    const degrade = degradeProfile({
        doc,
        pointerCoarse: device.coarse,
        windowed: p.windowed
    });
    return {
        mode,
        wrap: p.wrapLines && degrade.wrap,
        ariaLabel: `File: ${p.path ?? ""}`,
        degrade,
        fontPx: device.fontPx,
        noVirtualKeyboard: device.coarse && mode === "Read"
    };
}
/** React facade over the EditorEngine port; it never touches CodeMirror directly. */ const CodeEditor = /*#__PURE__*/ (0,react.forwardRef)(function CodeEditor(props, ref) {
    const { content, language, path, mode = "Read", wrapLines, gutterMarks, windowed, onReady } = props;
    const hostRef = (0,react.useRef)(null);
    const engineRef = (0,react.useRef)(null);
    const coarse = (0,useCoarsePointer/* useCoarsePointer */.O)();
    const fontPx = (0,fontSize/* useFontPx */.L9)(coarse);
    const live = (0,react.useRef)({
        props,
        device: {
            coarse,
            fontPx
        }
    });
    live.current = {
        props,
        device: {
            coarse,
            fontPx
        }
    };
    const notice = (0,react.useMemo)(()=>degradeProfile({
            doc: content,
            pointerCoarse: coarse,
            windowed
        }).notice, [
        content,
        coarse,
        windowed
    ]);
    // The engine mounts asynchronously (lazy chunk), so callers hold a stable delegate.
    const delegate = (0,react.useMemo)(()=>{
        const e = ()=>{
            if (!engineRef.current) throw new Error("CodeEditor: engine not ready (wait for onReady)");
            return engineRef.current;
        };
        return {
            get features () {
                return e().features;
            },
            createBufferState: (init)=>e().createBuffer({
                    ...init,
                    config: modeConfig(live.current.props, live.current.device, init.doc)
                }),
            getState: ()=>e().getBuffer(),
            setBufferState: (s)=>e().setBuffer(s),
            getText: ()=>e().getText(),
            getCursor: ()=>e().getCursor(),
            insertText: (t)=>e().insertText(t),
            gotoLine: (n)=>e().reveal(n),
            highlightRange: (r)=>e().highlight(r),
            getViewport: ()=>e().getViewport(),
            scrollSnapshot: ()=>e().scrollSnapshot(),
            restoreScroll: (s)=>e().restoreScroll(s),
            focus: ()=>e().focus(),
            setSource: (src)=>e().setSource(src),
            onNeedRange: (cb)=>e().onNeedRange(cb),
            provideRange: (r, t)=>e().provideRange(r, t),
            evictRange: (r)=>e().evictRange(r),
            takePending: ()=>e().takePending()
        };
    }, []);
    (0,react.useImperativeHandle)(ref, ()=>delegate, [
        delegate
    ]);
    // Mount once; later prop changes go through mode/buffer updates, never a rebuild.
    (0,react.useEffect)(()=>{
        let cancelled = false;
        let mounted = null;
        (async ()=>{
            const { CmEngine } = await Promise.all(/* import() */[__webpack_require__.e(9154), __webpack_require__.e(8513), __webpack_require__.e(918)]).then(__webpack_require__.bind(__webpack_require__, 40918));
            if (cancelled || !hostRef.current) return;
            const engine = new CmEngine();
            const { props: cur, device } = live.current;
            const config = modeConfig(cur, device, cur.content);
            const buffer = await engine.createBuffer({
                doc: cur.content,
                language: cur.language,
                gutterMarks: cur.gutterMarks,
                config
            });
            if (cancelled || !hostRef.current) return;
            mounted = engine.mount(hostRef.current, {
                ...config,
                buffer
            });
            engineRef.current = mounted;
            mounted.setMode(modeConfig(live.current.props, live.current.device, live.current.props.content)); // props may have moved while the chunk loaded
            live.current.props.onReady?.(delegate);
        })();
        return ()=>{
            cancelled = true;
            mounted?.destroy();
            engineRef.current = null;
        };
    }, [
        delegate
    ]);
    (0,react.useEffect)(()=>{
        engineRef.current?.setMode(modeConfig({
            mode,
            wrapLines,
            path,
            windowed
        }, {
            coarse,
            fontPx
        }, content));
    // The doc only decides wrapping here; a new document is handled by the buffer effect below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        mode,
        wrapLines,
        path,
        windowed,
        coarse,
        fontPx
    ]);
    // A read-only buffer follows its source; an editable one never has its doc replaced from props.
    const seeded = (0,react.useRef)({
        content,
        language,
        gutterMarks
    });
    (0,react.useEffect)(()=>{
        const prev = seeded.current;
        seeded.current = {
            content,
            language,
            gutterMarks
        };
        const engine = engineRef.current;
        if (!engine || mode === "Edit") return;
        if (prev.content === content && prev.language === language && prev.gutterMarks === gutterMarks) return;
        void engine.createBuffer({
            doc: content,
            language,
            gutterMarks,
            config: modeConfig({
                mode,
                wrapLines,
                path,
                windowed
            }, {
                coarse,
                fontPx
            }, content)
        }).then((b)=>engine.setBuffer(b));
    }, [
        content,
        language,
        gutterMarks,
        mode,
        wrapLines,
        path,
        windowed,
        coarse,
        fontPx
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: CodeEditor_css/* codeEditorRoot */.Lt,
        children: [
            notice && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: CodeEditor_css/* degradeNotice */.NA,
                role: "note",
                "data-testid": "degrade-notice",
                children: notice
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                ref: hostRef,
                className: CodeEditor_css/* codeEditor */.VK
            })
        ]
    });
});

// EXTERNAL MODULE: ./src/components/sessions/files/AccessibleView.tsx + 1 modules
var AccessibleView = __webpack_require__(34465);
// EXTERNAL MODULE: ./src/lib/files/analytics/filesAnalytics.ts
var filesAnalytics = __webpack_require__(86832);
;// ./src/components/sessions/files/useOpenLatency.ts
/* __next_internal_client_entry_do_not_use__ useOpenLatency auto */ 

/**
 * `files.perf.open_ms`: content in hand (RPC returned) to first paint, emitted once per open.
 * The clock starts at first render (the caller mounts when the content arrives); `restart` re-arms it.
 */ function useOpenLatency() {
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "FileContentViewer"
    });
    const startedAt = (0,react.useRef)(performance.now());
    const restart = (0,react.useCallback)(()=>{
        startedAt.current = performance.now();
    }, []);
    const finish = (0,react.useCallback)((labels)=>{
        const t0 = startedAt.current;
        if (t0 === null) return;
        startedAt.current = null;
        requestAnimationFrame(()=>analytics.emitTimed("files.perf.open_ms", performance.now() - t0, labels));
    }, [
        analytics
    ]);
    return (0,react.useMemo)(()=>({
            restart,
            finish
        }), [
        restart,
        finish
    ]);
}

;// ./src/components/sessions/files/WindowedFileView.tsx
/* __next_internal_client_entry_do_not_use__ WindowedFileView auto */ 









let instances = 0;
/** The editor handle the bridge sees: absolute line numbers, jumps that load their window first. */ function bridgeHandle(editor, file) {
    return {
        getText: ()=>editor.getText(),
        getCursor: ()=>editor.getCursor(),
        gotoLine: (n)=>{
            void file.goTo(n);
        },
        highlightRange: (r)=>{
            void file.settled().then(()=>editor.highlightRange(r));
        },
        scrollSnapshot: ()=>editor.scrollSnapshot(),
        restoreScroll: (s)=>editor.restoreScroll(s),
        focus: ()=>editor.focus(),
        windowed: file
    };
}
/** A windowed (T1/T2) file in Read mode: CodeMirror over a lazy document that loads line windows on demand. */ function WindowedFileView({ sessionId, path, source, language, accessible = false, dirty = false, gutterMarks, reloadKey, onReady, onDemoted }) {
    const transport = (0,useFileTransport/* useFileTransport */.J7)();
    const latency = useOpenLatency();
    const [current, setCurrent] = (0,react.useState)(null);
    // The file the open timer was armed for: in the commit where `path` changes, `state` still describes the previous file.
    const armedFor = (0,react.useRef)(null);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        latency.restart();
        const f = new windowedFile/* WindowedFile */.oK(transport, {
            sessionId,
            path
        }, source.size);
        armedFor.current = f;
        setCurrent({
            file: f,
            id: ++instances
        });
        return ()=>{
            f.dispose();
            setCurrent(null);
        };
    }, [
        transport,
        sessionId,
        path,
        source.size,
        reloadKey,
        latency
    ]);
    const file = current?.file ?? null;
    const state = (0,useWindowedState/* useWindowedState */.O)(file);
    // An open that fails has no window to show; the full read is the always-working floor, so fall back like a demotion.
    const { opened } = state;
    const { tier, size } = source;
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (opened && file === armedFor.current) latency.finish({
            size: (0,events/* bucketBytes */.gi)(size),
            degraded: "false",
            tier
        });
    }, [
        opened,
        file,
        latency,
        tier,
        size
    ]);
    const fallBack = state.demoted || state.openFailed;
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (fallBack) onDemoted?.();
    }, [
        fallBack,
        onDemoted
    ]);
    if (!file) return null;
    if (accessible) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(AccessibleView/* AccessibleView */.w, {
            path: path,
            windowed: file,
            dirty: dirty,
            saveDisabledReason: "saving is not available yet",
            onReady: onReady
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(LargeFileStates/* LargeFileStates */.oX, {
                file: file,
                state: state
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(WindowedEditor, {
                file: file,
                path: path,
                gutterMarks: gutterMarks,
                language: language,
                startLine: source.windowStartLine,
                onReady: onReady
            }, current?.id)
        ]
    });
}
function WindowedEditor({ file, path, language, startLine, gutterMarks, onReady }) {
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>()=>file.detach(), [
        file
    ]);
    // Marks set before attach seed the first buffer; later changes rebuild it in place.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        void file.setGutterMarks(gutterMarks);
    }, [
        file,
        gutterMarks
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(CodeEditor, {
        content: "",
        language: language,
        path: path,
        mode: "Read",
        windowed: true,
        onReady: (editor)=>{
            void file.attach(editor, language, startLine);
            onReady?.(bridgeHandle(editor, file));
        }
    });
}

// EXTERNAL MODULE: ./src/lib/files/pageSource.ts
var pageSource = __webpack_require__(28302);
;// ./src/components/sessions/files/ReadSurface.tsx
/* __next_internal_client_entry_do_not_use__ ReadSurface auto */ 








/** The file body in Read mode: CodeMirror, or the plain paged Accessible view when the device chose it. */ function ReadSurface({ accessible, dirty, content, language, path, wrapLines, gutterMarks, onReady }) {
    const source = (0,react.useMemo)(()=>accessible ? (0,pageSource/* textPageSource */.wd)(content) : null, [
        accessible,
        content
    ]);
    const coarse = (0,useCoarsePointer/* useCoarsePointer */.O)();
    const latency = useOpenLatency();
    const ready = (handle)=>{
        const degraded = degradeProfile({
            doc: content,
            pointerCoarse: coarse
        }).notice !== null;
        latency.finish({
            size: (0,events/* bucketBytes */.gi)(content.length),
            degraded: degraded ? "true" : "false",
            tier: "t0"
        });
        onReady?.(handle);
    };
    if (source) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(AccessibleView/* AccessibleView */.w, {
            path: path,
            source: source,
            dirty: dirty,
            saveDisabledReason: "saving is not available yet",
            onReady: ready
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(CodeEditor, {
        content: content,
        language: language,
        path: path,
        mode: "Read",
        wrapLines: wrapLines,
        gutterMarks: gutterMarks,
        onReady: ready
    });
}

// EXTERNAL MODULE: ./src/lib/utils/parseDiff.ts
var parseDiff = __webpack_require__(93020);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FFileContentViewer.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71XyXKjMBC9-yv6kipzIAV4mUS55E-mZCRs2SAxQo6xp_zvUxKbkAHHmclc7EJ0v-5-vYn3Az0nEme0gEv6ul8tAvg9A1DC_AEoiXmRCJkhkEJhReeLdUDo1nubAVxn19m7C1AYzeCpBtjg-LCV4siJn4uCKSY4gigIniCoIADCYFrat8Wvs-fKTmhUCCvyFJ8RJCkttYT-9wmTNK6UY5EeM67f7Cjb7hQy5vSz-KAyScUJwY4RQo1MZx_BB5Zz3_8ZvtJLJC8h8d4649GjxnHKttxnimYFgphyRaU-3h8LxZKzHwuuKFf2K9fdWKRCul5FJg9bnCMIo7y0PFwYDxPBlV-wC3UVFxtvFDQ0rzIst4wjzXsLunRBly-5CVzkOGbqjCB4Xlnyq2GaRsgwYdSAOSaE8S2CaFkdjIbfGlsbY59TDANb80ffTcZTxqm_SUV80AgnRtQOQVgDtompnzdCEioRRHkJhUgZcWxRr5Pylcj9QY8yW0piwo4FglWVesxZhquKapo0eF4XoN3EEhhPGGeKWhG9PES8KduT1PTr3zYXQS8T67ysi6z1cyOUEhmCcDJ0q14afXOU4Iyl55ucHr07nci84fZto3810U80jA3OBad2Gpvn-CgLrZ8L1vDUUhE0Mehi2VHJlGUdj1sPLUJOdRmtArvDNvdc73yAfsPH_fLvZWuUy7P3QDbL8ZFRDie6dY5UgyOlJYKwnz18VMJuK5sNOtiYd6q5mwB56VJ4cf0cnIpOFzoy63uTs6mk6a6wk2K2bI4l5Wqw9DpC0E4zN14kgYM9PGxe7OmXjKP1htKoRIu0_f9b0d4Z91fE7sH9xVz5cDW1VOjo4tzfANWtOSR8-ErVGyKc0M_9iXHz9vu6Q9FS-YTGQtZ7q52qf9s2FWF-ShPVzI6OuG_ojvShTTpRuZPDbyRHi9B2JTOuZLj0mxtJ3QT66OZ2u9nTWPkJ004IrjCzNyR357E1fSdXa_Vd4Nh3bburtGWvvlK1aPn3D4wv0z7YUovevfHXZxIS4zSem08c34wbG0B-_ZIedX1mSLFj7g2V94wShmFeLxPNJpYF9Yzp3oUJIGO89XtZX2-r0zrA5vDa6dJHdSfTZQEf_jHw9Q_xMA6t7A4AAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FFileContentViewer_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71XyXKjMBC9_yv6kipzIAV4mUS55E_mZCRs2SAxQo6xp_zvUxKbkAHHmclc7EJ0v_5_vYn3Az0nEme0gEv6ul8tAvg9A1DC_AEoiXmRCJkhkEJhReeLdUDo1nubAVxn19m7C1AYzeCpBtjg_LCV4siJn4uCKSY4gigIniCoIADCYFrat8Wvs_fKTmhUCCvyFJ8RJCkttYT_9wmTNK6UY5EeM67f7Cjb7hQy5vSz_KAyScUJwY4RQo1MZx_BB5Zz3_8ZvtJLJC8h8d4649GjxnHKttxnimYFgphyRaU_3h8LxZKzHwuuKFf2K9fdWKRCul5FJg9bnCMIo7y0PFwYDxPBlV_wC3UVFxtvFDQ0rzIst4wjzXsLunRBly_5CVzkOGbqjCB4Xlnyq2GaRsgwYdSAOSaE8S2CaFkdjIbfGlsbY59TDANb80ffTcZTxqm_SUV80AgnRtQOQVgDtompnzdCEioRRHkJhUgZcWxRr5Pylcj9QY8yW0piwo4FglWVesxZhquKapo0eF4XoN3EEhhPGGeKWhG9PES8KduT1PTr3zYXQS8T67ysi6z1cyOUEhmCcDJ0q14afXOU4Iyl55ucHr07nci84fZto3810U80jA3OBad2Gpvn_CgLrZ8L1vDUUhE0Mehi2VHJlGUdj1sPLUJOdRmtArvDNvdc73yAfsPH_fLvZWuUy7P3QDbL8ZFRDie6dY5UgyOlJYKwnz18VMJuK5sNOtiYd6q5mwB56VJ4cf0cnIpOFzoy63uTs6mk6a6wk2K2bI4l5Wqw9DpC0E4zN14kgYM9PGxe7OmXjKP1htKoRIu0_f9b0d4Z91fE7sH9xVz5cDW1VOjo4tzfANWtOSR8_ErVGyKc0M_9iXHz9vu6Q9FS_YTGQtZ7q52qf9s2FWF_ShPVzI6OuG_ojvShTTpRuZPDbyRHi9B2JTOuZLj0mxtJ3QT66OZ2u9nTWPkJ004IrjCzNyR357E1fSdXa_Vd4Nh3bburtGWvvlK1aPn3D4wv0z7YUovevfHXZxIS4zSem08c34wbG0B_ZIedX1mSLFj7g2V94wShmFeLxPNJpYF9Yzp3oUJIGO89XtZX2_r0zrA5vDa6dJHdSfTZQEf_jHw9Q_xMA6t7A4AAA_22_7D = __webpack_require__(44632);
;// ./src/components/sessions/FileContentViewer.css.ts


var binaryIcon = 'zl9j53h';
var binaryMeta = 'zl9j53j';
var binaryPlaceholder = 'zl9j53g';
var binaryTitle = 'zl9j53i';
var breadcrumb = 'zl9j538';
var breadcrumbCurrent = 'zl9j53a';
var breadcrumbSegment = 'zl9j539';
var breadcrumbSep = 'zl9j53b';
var container = 'zl9j531';
var downloadButton = 'zl9j53k';
var emptyHint = 'zl9j533';
var emptyIcon = 'zl9j534';
var emptyState = 'zl9j532';
var FileContentViewer_css_error = 'zl9j536';
var imagePreview = 'zl9j53m';
var imageViewer = 'zl9j53l';
var FileContentViewer_css_loading = 'zl9j535';
var pdfEmbed = 'zl9j53o';
var pdfViewer = 'zl9j53n';
var spinner = 'zl9j537';
var truncationWarning = 'zl9j53c';
var videoMeta = 'zl9j53r';
var videoPlayer = 'zl9j53q';
var videoViewer = 'zl9j53p';
var viewer = 'zl9j53d';
var wrapToggleButton = 'zl9j53e';
var wrapToggleButtonActive = 'zl9j53f';
;// ./src/components/sessions/FileContentViewer.tsx
/* __next_internal_client_entry_do_not_use__ DOUBLE_G_WINDOW_MS,isDoubleGPress,detectLanguage,FileContentViewer auto */ 








// Language detection map: file extension → CodeMirror language ID.
const EXT_TO_LANG = {
    go: "go",
    ts: "typescript",
    tsx: "tsx",
    js: "javascript",
    jsx: "jsx",
    py: "python",
    rb: "ruby",
    rs: "rust",
    java: "java",
    kt: "kotlin",
    cs: "csharp",
    cpp: "cpp",
    cc: "cpp",
    c: "c",
    h: "c",
    hpp: "cpp",
    swift: "swift",
    php: "php",
    html: "html",
    htm: "html",
    css: "css",
    scss: "scss",
    sass: "sass",
    less: "less",
    json: "json",
    yaml: "yaml",
    yml: "yaml",
    toml: "toml",
    md: "markdown",
    markdown: "markdown",
    sh: "bash",
    bash: "bash",
    zsh: "bash",
    fish: "fish",
    sql: "sql",
    xml: "xml",
    graphql: "graphql",
    gql: "graphql",
    proto: "proto",
    tf: "hcl",
    hcl: "hcl",
    r: "r",
    lua: "lua",
    pl: "perl",
    ex: "elixir",
    exs: "elixir",
    erl: "erlang",
    hs: "haskell",
    clj: "clojure",
    dockerfile: "dockerfile",
    makefile: "makefile",
    mk: "makefile",
    diff: "diff",
    patch: "diff"
};
function detectLanguage(filePath) {
    const base = filePath.split("/").pop() || "";
    const lower = base.toLowerCase();
    // Check full filename first (Dockerfile, Makefile, etc.)
    if (lower === "dockerfile") return "dockerfile";
    if (lower === "makefile") return "makefile";
    if (lower === ".gitignore") return "ini";
    if (lower === ".env" || lower === ".envrc") return "ini";
    const ext = lower.split(".").pop() || "";
    return EXT_TO_LANG[ext] || "text";
}
function Breadcrumb({ path, onSegmentClick, downloadUrl, openUrl, wrapLines, onToggleWrap }) {
    const segments = path.split("/").filter(Boolean);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: breadcrumb,
        children: [
            segments.map((seg, i)=>{
                const segPath = segments.slice(0, i + 1).join("/");
                const isLast = i === segments.length - 1;
                return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                    children: [
                        isLast ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            className: breadcrumbCurrent,
                            title: segPath,
                            children: seg
                        }) : // Breadcrumb navigation is presentational, no tracked event.
                        // analytics-exempt
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            className: breadcrumbSegment,
                            onClick: onSegmentClick ? ()=>onSegmentClick(segPath) : undefined,
                            title: segPath,
                            type: "button",
                            children: seg
                        }),
                        !isLast && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            className: breadcrumbSep,
                            children: "/"
                        })
                    ]
                }, segPath);
            }),
            onToggleWrap && // View-only wrap toggle, no tracked event.
            // analytics-exempt
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                className: [
                    wrapToggleButton,
                    wrapLines ? wrapToggleButtonActive : ""
                ].filter(Boolean).join(" "),
                onClick: onToggleWrap,
                title: wrapLines ? "Disable line wrap" : "Enable line wrap",
                children: "↵ Wrap"
            }),
            openUrl && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("a", {
                href: openUrl,
                target: "_blank",
                rel: "noopener noreferrer",
                className: downloadButton,
                title: "Open file in browser",
                children: "↗ Open"
            }),
            downloadUrl && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("a", {
                href: downloadUrl,
                download: true,
                className: downloadButton,
                title: "Download file",
                children: "↓ Download"
            })
        ]
    });
}
// ---- Image content types ----
const IMAGE_CONTENT_TYPES = new Set([
    "image/png",
    "image/jpeg",
    "image/gif",
    "image/svg+xml",
    "image/webp",
    "image/bmp"
]);
const PDF_CONTENT_TYPES = new Set([
    "application/pdf"
]);
const VIDEO_CONTENT_TYPES = new Set([
    "video/mp4",
    "video/webm",
    "video/quicktime",
    "video/ogg",
    "application/ogg"
]);
// Strip parameters (e.g. "; charset=utf-8") and normalize case before matching.
function matchesContentType(contentType, types) {
    if (!contentType) return false;
    const base = contentType.split(";")[0].trim().toLowerCase();
    return types.has(base);
}
// +feature: file-content-viewer
function FileContentViewer({ sessionId, filePath, baseUrl, diffContent, reloadKey, onEditorReady, accessible = false, dirty = false, source }) {
    const [demoted, setDemoted] = (0,react.useState)(false);
    const windowed = source?.kind === "windowed" && !demoted ? source : null;
    const { data, loading, error } = (0,useFileService/* useGetFileContent */.Li)(sessionId, windowed ? null : filePath, baseUrl, reloadKey);
    const [wrapLines, setWrapLines] = (0,react.useState)(false);
    const gutterMarks = (0,react.useMemo)(()=>diffContent && filePath ? (0,parseDiff/* buildGutterMarks */.g)(diffContent, filePath) : new Map(), [
        diffContent,
        filePath
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        setWrapLines(false);
        setDemoted(false);
    }, [
        filePath
    ]);
    if (!filePath) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: emptyState,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                    className: emptyIcon,
                    children: "\uD83D\uDCC4"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                    children: "Select a file to view its contents"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                    className: emptyHint,
                    children: "Press ⌘P (or Ctrl+P) to quick-open any file"
                })
            ]
        });
    }
    const rawUrl = `/api/files/raw?sessionId=${encodeURIComponent(sessionId)}&path=${encodeURIComponent(filePath)}`;
    const downloadUrl = `${rawUrl}&download=true`;
    const openUrl = rawUrl;
    if (windowed) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: container,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Breadcrumb, {
                    path: filePath,
                    openUrl: openUrl,
                    downloadUrl: downloadUrl
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: viewer,
                    children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(WindowedFileView, {
                        sessionId: sessionId,
                        path: filePath,
                        source: windowed,
                        language: detectLanguage(filePath),
                        accessible: accessible,
                        dirty: dirty,
                        gutterMarks: gutterMarks.size > 0 ? gutterMarks : undefined,
                        reloadKey: reloadKey,
                        onReady: onEditorReady && ((handle)=>onEditorReady(handle, filePath)),
                        onDemoted: ()=>setDemoted(true)
                    })
                })
            ]
        });
    }
    if (loading) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: container,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Breadcrumb, {
                    path: filePath
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                    className: FileContentViewer_css_loading,
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            className: spinner
                        }),
                        "Loading file…"
                    ]
                })
            ]
        });
    }
    if (error) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: container,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Breadcrumb, {
                    path: filePath
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: FileContentViewer_css_error,
                    children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                        children: [
                            "⚠ ",
                            error
                        ]
                    })
                })
            ]
        });
    }
    if (!data) return null;
    // Inline image rendering for known image content types.
    const isImage = data.isBinary && matchesContentType(data.contentType, IMAGE_CONTENT_TYPES);
    if (isImage) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: container,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Breadcrumb, {
                    path: filePath,
                    openUrl: openUrl,
                    downloadUrl: downloadUrl
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: imageViewer,
                    children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("img", {
                        src: rawUrl,
                        alt: filePath,
                        className: imagePreview
                    })
                })
            ]
        });
    }
    const isPdf = data.isBinary && matchesContentType(data.contentType, PDF_CONTENT_TYPES);
    if (isPdf) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: container,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Breadcrumb, {
                    path: filePath,
                    openUrl: openUrl,
                    downloadUrl: downloadUrl
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: pdfViewer,
                    children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("embed", {
                        src: `${rawUrl}#view=FitH&navpanes=0`,
                        type: "application/pdf",
                        className: pdfEmbed,
                        title: filePath
                    })
                })
            ]
        });
    }
    const isVideo = data.isBinary && matchesContentType(data.contentType, VIDEO_CONTENT_TYPES);
    if (isVideo) {
        const sizeKb = Number(data.size) / 1024;
        const sizeLabel = sizeKb >= 1024 ? `${(sizeKb / 1024).toFixed(1)} MB` : `${sizeKb.toFixed(1)} KB`;
        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: container,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Breadcrumb, {
                    path: filePath,
                    openUrl: openUrl,
                    downloadUrl: downloadUrl
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                    className: videoViewer,
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("video", {
                            src: rawUrl,
                            controls: true,
                            preload: "metadata",
                            className: videoPlayer,
                            children: "Your browser does not support the video element."
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                            className: videoMeta,
                            children: [
                                filePath.split("/").pop(),
                                " \xb7 ",
                                sizeLabel,
                                data.contentType ? ` · ${data.contentType}` : ""
                            ]
                        })
                    ]
                })
            ]
        });
    }
    if (data.isBinary) {
        const sizeKb = Number(data.size) / 1024;
        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: container,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Breadcrumb, {
                    path: filePath,
                    openUrl: openUrl,
                    downloadUrl: downloadUrl
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                    className: binaryPlaceholder,
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            className: binaryIcon,
                            children: "\uD83D\uDD12"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                            className: binaryTitle,
                            children: "Binary file — cannot display"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                            className: binaryMeta,
                            children: [
                                sizeKb >= 1024 ? `${(sizeKb / 1024).toFixed(1)} MB` : `${sizeKb.toFixed(1)} KB`,
                                data.contentType ? ` · ${data.contentType}` : ""
                            ]
                        })
                    ]
                })
            ]
        });
    }
    const lang = detectLanguage(filePath);
    const hasGutterMarks = gutterMarks.size > 0;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: container,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Breadcrumb, {
                path: filePath,
                openUrl: openUrl,
                downloadUrl: downloadUrl,
                wrapLines: wrapLines,
                onToggleWrap: ()=>setWrapLines((w)=>!w)
            }),
            demoted && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(LargeFileStates/* FallbackChip */.dq, {}),
            data.isTruncated && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: truncationWarning,
                children: "⚠ File truncated to 1 MB — only the first portion is shown"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: viewer,
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ReadSurface, {
                    accessible: accessible,
                    dirty: dirty,
                    content: data.content,
                    language: lang,
                    path: filePath,
                    wrapLines: wrapLines,
                    gutterMarks: hasGutterMarks ? gutterMarks : undefined,
                    onReady: onEditorReady && ((handle)=>onEditorReady(handle, filePath))
                }, `${filePath}:${reloadKey ?? 0}:${accessible}`)
            })
        ]
    });
}


/***/ }),

/***/ 24293:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

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

/***/ 28302:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ju: () => (/* binding */ pageFromLines),
/* harmony export */   Ne: () => (/* binding */ pageOfLine),
/* harmony export */   hX: () => (/* binding */ paginateLines),
/* harmony export */   pi: () => (/* binding */ ACCESSIBLE_PAGE_SIZE),
/* harmony export */   wd: () => (/* binding */ textPageSource)
/* harmony export */ });
/* unused harmony export pageCount */
/** Where the Accessible view gets its lines; Task 1.6.5c swaps in a transport-backed source for windowed buffers. */ const ACCESSIBLE_PAGE_SIZE = 500;
/** A source over an in-memory string: line starts are indexed once, lines are sliced per page (no per-line copy). */ function textPageSource(text) {
    const starts = [
        0
    ];
    for(let i = text.indexOf("\n"); i !== -1; i = text.indexOf("\n", i + 1))starts.push(i + 1);
    const lineEnd = (i)=>i + 1 < starts.length ? starts[i + 1] - 1 : text.length;
    return {
        lineCount: starts.length,
        text: ()=>text,
        lines (start, end) {
            const out = [];
            for(let i = Math.max(0, start); i < Math.min(end, starts.length); i++){
                const e = lineEnd(i);
                out.push(text.slice(starts[i], e > starts[i] && text.charCodeAt(e - 1) === 13 ? e - 1 : e));
            }
            return out;
        }
    };
}
const pageCount = (total, size = ACCESSIBLE_PAGE_SIZE)=>Math.max(1, Math.ceil(total / size));
const pageOfLine = (line, size = ACCESSIBLE_PAGE_SIZE)=>Math.floor((line - 1) / size);
/** A page from lines the caller already read (windowed files); until `exact`, the total is an estimate. */ function pageFromLines(lines, page, total, exact, size = ACCESSIBLE_PAGE_SIZE) {
    const pages = pageCount(total, size);
    const first = Math.min(Math.max(0, page), pages - 1) * size + 1;
    const last = first + lines.length - 1;
    const shown = lines.length > 0 ? last : Math.min(first + size - 1, total); // an unloaded page still names its range
    return {
        first,
        last,
        total,
        pages,
        lines,
        label: `Lines ${first}-${shown} of ${exact ? "" : "about "}${total}`
    };
}
/** The `page`-th (0-based) page of at most `size` lines; an out-of-range page clamps to the last. */ function paginateLines(source, page, size = ACCESSIBLE_PAGE_SIZE) {
    const total = source.lineCount;
    const pages = pageCount(total, size);
    const p = Math.min(Math.max(0, page), pages - 1);
    const start = p * size;
    const lines = source.lines(start, start + size);
    const first = start + 1;
    const last = start + lines.length;
    return {
        first,
        last,
        total,
        pages,
        lines,
        label: `Lines ${first}-${last} of ${total}`
    };
}


/***/ }),

/***/ 30574:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ec: () => (/* binding */ buildLineStatsMap),
/* harmony export */   lT: () => (/* binding */ buildGitStatusMap)
/* harmony export */ });
/* unused harmony export fileChangeToStatusLetter */
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(29238);

/**
 * Maps a FileStatus to the single-character badge shown in compact file-tree
 * rows. The conflict glyph here ("U", for "Unmerged" — matching git's own
 * porcelain status char) intentionally differs from VcsPanel.tsx's
 * FILE_STATUS_META, which uses "!!" for the same status in its full-row
 * panel context where a louder glyph is appropriate.
 */ function fileChangeToStatusLetter(status) {
    switch(status){
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .FileStatus */ .BM.MODIFIED:
            return "M";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .FileStatus */ .BM.ADDED:
            return "A";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .FileStatus */ .BM.DELETED:
            return "D";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .FileStatus */ .BM.RENAMED:
            return "R";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .FileStatus */ .BM.COPIED:
            return "C";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .FileStatus */ .BM.UNTRACKED:
            return "?";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .FileStatus */ .BM.IGNORED:
            return "!";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .FileStatus */ .BM.CONFLICT:
            return "U";
        default:
            return "";
    }
}
function buildGitStatusMap(files) {
    const map = new Map();
    for (const f of files){
        const letter = fileChangeToStatusLetter(f.status);
        if (letter && f.path) {
            map.set(f.path, letter);
        }
    }
    return map;
}
/** Maps a file path to its added/removed line counts, from FileChange.additions/deletions. */ function buildLineStatsMap(files) {
    const map = new Map();
    for (const f of files){
        if (f.path && (f.additions > 0 || f.deletions > 0)) {
            map.set(f.path, {
                add: f.additions,
                del: f.deletions
            });
        }
    }
    return map;
}


/***/ }),

/***/ 32723:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   X: () => (/* binding */ bannerAction),
/* harmony export */   v: () => (/* binding */ banner)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(13507);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2Ffiles_2FStaleBanner_css_ts_vanilla_css_22_2C_22source_22_3A_22LmNvM243NDAgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBqdXN0aWZ5LWNvbnRlbnQ6IHNwYWNlLWJldHdlZW47CiAgZ2FwOiA4cHg7CiAgcGFkZGluZzogNnB4IDEwcHg7CiAgZm9udC1zaXplOiAxM3B4OwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6eSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6eik7CiAgYm9yZGVyLWJvdHRvbTogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6eCk7Cn0KLmNvM243NDEgewogIGJvcmRlcjogbm9uZTsKICBiYWNrZ3JvdW5kOiB0cmFuc3BhcmVudDsKICBjb2xvcjogaW5oZXJpdDsKICBmb250OiBpbmhlcml0OwogIHRleHQtZGVjb3JhdGlvbjogdW5kZXJsaW5lOwogIGN1cnNvcjogcG9pbnRlcjsKICBwYWRkaW5nOiAwIDJweDsKfQpAbWVkaWEgKHBvaW50ZXI6IGNvYXJzZSkgewogIC5jbzNuNzQxIHsKICAgIG1pbi1oZWlnaHQ6IDQ0cHg7CiAgICBtaW4td2lkdGg6IDQ0cHg7CiAgfQp9_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(58293);


var banner = 'co3n740';
var bannerAction = 'co3n741';

/***/ }),

/***/ 34465:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  o: () => (/* binding */ AccessibleSkipLink),
  w: () => (/* binding */ AccessibleView)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ./src/lib/files/pageSource.ts
var pageSource = __webpack_require__(28302);
// EXTERNAL MODULE: ./src/lib/files/useWindowedState.ts
var useWindowedState = __webpack_require__(61991);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2Ffiles%2FAccessibleView.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61U267aMBB85yv2pTog1ShQbvVRpf5GdVQdOfGGrHDsyN5wq_j3Kk6AhJZWqvoCaL2X2dkZpu-L1bJYhwR-jAA0hcqok4Tc4PF1BPFbaPKYMTkrIXOmLm3zUpIVBdK2YAnJ6-gymnadZn_odPCqktB8NiFlaGsFMZZBQoaW0TfhbZOzqWJVpbQmu5WwqI7XWOq8Ri9Sx-xKCbPqCMEZ0rBXfizE--wznuf-jJPYwAVqoQembHdqYuyqiBkgVdlu611ttXyoXsfqzBnnH5-SSX_deVy3YeNAmgsJi0ULs0_QNZY7yxLIFuiJB_slMJu3OVntQzO0ctQy0pslc5fVQewpUGowTnY1G7IoYf6EBxM36dKEy_OAHLMHnd-UJyU0BZUa1F9e2Nf48v0j3Edf39qplcqITxKS6bIPWmOuasP91p9uBD2y0RG2xvJv3HSXL5XfkhUGc5awGm6wiGOaJmLf7GJZ2LpET5kEVmltlG8CoV-zvNccOmyrZCDmVQs-zu00c4O1ud9U5Kokc3pUyrye3DICnVHCrFs93uLKx6zl8FAQowiVylBC5VFcfeL26HPjDp19lD0dCvQ4wPn_lCEepLEe-jk1LtsN3v91dK_Fm1asREF8FV7s8tydm0H5Jmbfna7S4EzNGImOWulOFY3fXeAsyGo8Slg-M-tvvPmLRtkrG3LnS9n-NIrx21jMk-TDEGJLUgTaq7HOxjN-LVGTgnHn-eZ_VvmAk5g_NNJQTqsW12V0-QlALDUpyQUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2Ffiles_2FAccessibleView_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61U267aMBB85yv2pTog1ShQbvVRpf5GdVQdOfGGrHDsyN5wq_j3Kk6AhJZWqvoCaL2X2dkZpu_L1bJYhwR_jAA0hcqok4Tc4PF1BPFbaPKYMTkrIXOmLm3zUpIVBdK2YAnJ6_gymnadZn_odPCqktB8NiFlaGsFMZZBQoaW0TfhbZOzqWJVpbQmu5WwqI7XWOq8Ri9Sx_xKCbPqCMEZ0rBXfizE_wznuf_jJPYwAVqoQembHdqYuyqiBkgVdlu611ttXyoXsfqzBnnH5_SSX_deVy3YeNAmgsJi0ULs0_QNZY7yxLIFuiJB_slMJu3OVntQzO0ctQy0pslc5fVQewpUGowTnY1G7IoYf6EBxM36dKEy_OAHLMHnd_UJyU0BZUa1F9e2Nf48v0j3Edf39qplcqITxKS6bIPWmOuasP91p9uBD2y0RG2xvJv3HSXL5XfkhUGc5awGm6wiGOaJmLf7GJZ2LpET5kEVmltlG8CoV_zvNccOmyrZCDmVQs_zu00c4O1ud9U5Kokc3pUyrye3DICnVHCrFs93uLKx6zl8FAQowiVylBC5VFcfeL26HPjDp19lD0dCvQ4wPn_lCEepLEe_jk1LtsN3v91dK_Fm1asREF8FV7s8tydm0H5Jmbfna7S4EzNGImOWulOFY3fXeAsyGo8Slg_M_tvvPmLRtkrG3LnS9n_NIrx21jMk_TDEGJLUgTaq7HOxjN_LVGTgnHn_eZ_VvmAk5g_NNJQTqsW12V0_QlALDUpyQUAAA_22_7D = __webpack_require__(86510);
;// ./src/components/sessions/files/AccessibleView.css.ts


var control = '_465h7s2';
var gotoInput = '_465h7s3';
var line = '_465h7s7';
var page = '_465h7s6';
var pageLabel = '_465h7s4';
var pager = '_465h7s1';
var region = '_465h7s0';
var skipLink = '_465h7s8';
var unsaved = '_465h7s5';
// EXTERNAL MODULE: ./src/styles/a11y.css.ts
var a11y_css = __webpack_require__(81956);
;// ./src/components/sessions/files/AccessibleView.tsx
/* __next_internal_client_entry_do_not_use__ AccessibleView,AccessibleSkipLink auto */ 





const TOUCH = {
    minHeight: 44,
    minWidth: 44
};
const baseName = (path)=>path.slice(path.lastIndexOf("/") + 1);
const EMPTY_SOURCE = (0,pageSource/* textPageSource */.wd)("");
/** Pages of a windowed file: one transport read per page, with loading and failed states. */ function useRemotePage(windowed, pageIndex) {
    const state = (0,useWindowedState/* useWindowedState */.O)(windowed);
    const [remote, setRemote] = (0,react.useState)(null);
    const [attempt, setAttempt] = (0,react.useState)(0);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!windowed) return;
        const ctl = new AbortController();
        setRemote({
            page: pageIndex,
            status: "loading"
        });
        windowed.pageLines(pageIndex * pageSource/* ACCESSIBLE_PAGE_SIZE */.pi + 1, pageSource/* ACCESSIBLE_PAGE_SIZE */.pi, ctl.signal).then((lines)=>setRemote({
                page: pageIndex,
                status: "ready",
                lines
            }), ()=>{
            if (!ctl.signal.aborted) setRemote({
                page: pageIndex,
                status: "failed"
            });
        });
        return ()=>ctl.abort();
    }, [
        windowed,
        pageIndex,
        attempt
    ]);
    const here = remote?.page === pageIndex ? remote : null;
    const lines = here?.status === "ready" ? here.lines : [];
    return {
        total: state.lineCount,
        page: (0,pageSource/* pageFromLines */.Ju)(lines, pageIndex, state.lineCount, state.exact),
        status: here?.status ?? "loading",
        retry: ()=>setAttempt((a)=>a + 1)
    };
}
/** Plain, fully rendered, read-only pages of 500 numbered lines: the guaranteed screen-reader read path (ux.md 8.10). */ function AccessibleView({ path, source = EMPTY_SOURCE, windowed, dirty = false, onSave, saveDisabledReason, onEditRequested, onReady }) {
    const [pageIndex, setPageIndex] = (0,react.useState)(0);
    const [tick, setTick] = (0,react.useState)(0);
    const [goto, setGoto] = (0,react.useState)("");
    const [announce, setAnnounce] = (0,react.useState)("");
    const [hitLine, setHitLine] = (0,react.useState)(null);
    const pre = (0,react.useRef)(null);
    const pending = (0,react.useRef)(null);
    const lastLine = (0,react.useRef)(1);
    const reasonId = (0,react.useId)();
    const remote = useRemotePage(windowed, pageIndex);
    const totalLines = windowed ? remote.total : source.lineCount;
    const current = windowed ? remote.page : (0,pageSource/* paginateLines */.hX)(source, pageIndex);
    const pageReady = !windowed || remote.status === "ready";
    // An estimated count can overshoot; once the exact one is known the page index follows it.
    const lastPage = current.pages - 1;
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (windowed && pageIndex > lastPage) setPageIndex(lastPage);
    }, [
        windowed,
        pageIndex,
        lastPage
    ]);
    const goToLine = (0,react.useCallback)((requested)=>{
        if (!Number.isFinite(requested)) return;
        const n = Math.min(Math.max(1, Math.trunc(requested)), totalLines);
        pending.current = {
            kind: "line",
            line: n
        };
        setPageIndex((0,pageSource/* pageOfLine */.Ne)(n));
        setTick((t)=>t + 1);
    }, [
        totalLines
    ]);
    const changePage = (to)=>{
        pending.current = {
            kind: "page"
        };
        setPageIndex(to);
    };
    const failedFirst = pageIndex * pageSource/* ACCESSIBLE_PAGE_SIZE */.pi + 1;
    const failedText = `Couldn't load lines ${failedFirst}-${Math.min(failedFirst + pageSource/* ACCESSIBLE_PAGE_SIZE */.pi - 1, totalLines)}. Retry`;
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (windowed && remote.status === "failed") setAnnounce(failedText);
    }, [
        windowed,
        remote.status,
        failedText
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!pageReady) return;
        const p = pending.current;
        // A jump aimed at another page waits for that page (this run may be the old page finishing its load).
        if (p?.kind === "line" && (0,pageSource/* pageOfLine */.Ne)(p.line) !== pageIndex) return;
        pending.current = null;
        if (!p) return;
        if (p.kind === "page") {
            pre.current?.focus();
            setAnnounce(current.label);
            return;
        }
        const line = windowed ? Math.min(p.line, current.last) : p.line;
        pre.current?.querySelector(`[data-line="${line}"]`)?.focus();
        lastLine.current = line;
        setAnnounce(`Line ${line}`);
    // Only a navigation (page or tick) may announce; `current.label` follows from pageIndex.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        pageIndex,
        tick,
        pageReady
    ]);
    const latest = (0,react.useRef)({
        goToLine,
        source
    });
    latest.current = {
        goToLine,
        source
    };
    const windowedRef = (0,react.useRef)(windowed);
    windowedRef.current = windowed;
    const readyRef = (0,react.useRef)(onReady);
    readyRef.current = onReady;
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        readyRef.current?.({
            getText: ()=>latest.current.source.text(),
            getCursor: ()=>({
                    line: lastLine.current,
                    col: 0
                }),
            gotoLine: (n)=>latest.current.goToLine(n),
            highlightRange: (r)=>setHitLine(r ? r.line : null),
            scrollSnapshot: ()=>({
                    kind: "scroll-snapshot"
                }),
            restoreScroll: ()=>undefined,
            focus: ()=>pre.current?.focus(),
            windowed: windowedRef.current
        });
    }, []);
    const submitGoto = (e)=>{
        e.preventDefault();
        goToLine(Number.parseInt(goto, 10));
    };
    const saveBlocked = !onSave;
    const name = `${baseName(path)}, accessible view${dirty ? ", unsaved changes" : ""}`;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("section", {
        className: region,
        "aria-label": name,
        "data-testid": "accessible-view",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("nav", {
                className: pager,
                "aria-label": "Pager",
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                        type: "button",
                        className: control,
                        style: TOUCH,
                        "aria-disabled": pageIndex === 0,
                        onClick: ()=>pageIndex > 0 && changePage(pageIndex - 1),
                        children: "Previous page"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: pageLabel,
                        "data-testid": "accessible-view-label",
                        children: current.label
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                        type: "button",
                        className: control,
                        style: TOUCH,
                        "aria-disabled": pageIndex >= current.pages - 1,
                        onClick: ()=>pageIndex < current.pages - 1 && changePage(pageIndex + 1),
                        children: "Next page"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("form", {
                        onSubmit: submitGoto,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                                children: [
                                    "Go to line",
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                        className: gotoInput,
                                        style: {
                                            minHeight: 44
                                        },
                                        type: "number",
                                        inputMode: "numeric",
                                        min: 1,
                                        max: totalLines,
                                        value: goto,
                                        onChange: (e)=>setGoto(e.target.value)
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                type: "submit",
                                className: control,
                                style: TOUCH,
                                children: "Go"
                            })
                        ]
                    }),
                    dirty && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: unsaved,
                        children: "Unsaved changes"
                    }),
                    dirty && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                type: "button",
                                className: control,
                                style: TOUCH,
                                "aria-disabled": saveBlocked,
                                "aria-describedby": saveBlocked ? reasonId : undefined,
                                onClick: ()=>onSave?.(),
                                children: "Save"
                            }),
                            saveBlocked && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                id: reasonId,
                                className: a11y_css/* visuallyHidden */.Q,
                                children: saveDisabledReason ?? "saving is not available"
                            })
                        ]
                    }),
                    onEditRequested && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                        type: "button",
                        className: control,
                        style: TOUCH,
                        onClick: ()=>onEditRequested(lastLine.current),
                        children: "Edit"
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: a11y_css/* visuallyHidden */.Q,
                role: "status",
                "aria-live": "polite",
                "data-testid": "accessible-view-announce",
                children: announce
            }),
            windowed && remote.status === "failed" && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                className: control,
                style: TOUCH,
                onClick: remote.retry,
                children: "Retry"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("pre", {
                ref: pre,
                "aria-busy": windowed && remote.status === "loading" ? true : undefined,
                className: page,
                tabIndex: 0,
                "aria-label": current.label,
                "data-testid": "accessible-view-page",
                onFocus: (e)=>{
                    const n = Number(e.target.dataset.line);
                    if (n) lastLine.current = n;
                },
                children: current.lines.map((text, i)=>{
                    const n = current.first + i;
                    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: line,
                        tabIndex: -1,
                        "data-line": n,
                        "data-hit": n === hitLine || undefined,
                        "data-testid": "accessible-view-line",
                        children: `${n} ${text}`
                    }, n);
                })
            })
        ]
    });
}
/** First focusable element of the editor region: jumps between the two read views. */ function AccessibleSkipLink({ on, onToggle }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
        type: "button",
        className: skipLink,
        "data-testid": "accessible-skip-link",
        onClick: onToggle,
        children: on ? "Switch to code editor" : "Switch to accessible view"
    });
}


/***/ }),

/***/ 37353:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   JB: () => (/* binding */ invalidateFileContent),
/* harmony export */   Li: () => (/* binding */ useGetFileContent),
/* harmony export */   Tl: () => (/* binding */ fetchDirectoryFiles),
/* harmony export */   _K: () => (/* binding */ searchFiles)
/* harmony export */ });
/* unused harmony export fetchFileContent */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(4435);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(31381);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(73396);
/* __next_internal_client_entry_do_not_use__ invalidateFileContent,fetchDirectoryFiles,searchFiles,fetchFileContent,useGetFileContent auto */ 



const fileContentCache = new Map();
const FILE_CONTENT_CACHE_TTL_MS = 30000;
/** Drops a cached read so the next fetch hits the server (reload after an on-disk change). */ function invalidateFileContent(sessionId, filePath) {
    fileContentCache.delete(`${sessionId}:${filePath}`);
}
/**
 * createFileClient creates a ConnectRPC client for the SessionService.
 * Extracted as a helper so both hooks share the same pattern.
 */ function createFileClient(_baseUrl) {
    return (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_3__/* .getConnectTransport */ .qW)());
}
/**
 * fetchDirectoryFiles is a standalone async function for listing files.
 * Components can call this directly or via the hook.
 */ async function fetchDirectoryFiles(sessionId, path, includeIgnored, baseUrl) {
    const client = createFileClient(baseUrl);
    return await client.listFiles({
        sessionId,
        path: path || ".",
        includeIgnored
    });
}
/**
 * searchFiles calls the SearchFiles RPC and returns matching files with full paths.
 */ async function searchFiles(sessionId, query, includeIgnored, baseUrl) {
    const client = createFileClient(baseUrl);
    return await client.searchFiles({
        sessionId,
        query,
        includeIgnored,
        maxResults: 500
    });
}
/**
 * fetchFileContent is a standalone async function for fetching file content.
 */ async function fetchFileContent(sessionId, path, baseUrl) {
    const client = createFileClient(baseUrl);
    return await client.getFileContent({
        sessionId,
        path
    });
}
/**
 * useGetFileContent fetches the content of a file whenever filePath changes.
 * Returns null data while loading or if filePath is null.
 */ function useGetFileContent(sessionId, filePath, baseUrl, /** Bump to refetch the same file; pair it with `invalidateFileContent`. */ reloadKey = 0) {
    // Tagged with its source so a render for a new path never sees the previous file's content (the effect below
    // only replaces it after that render commits).
    const [loaded, setLoaded] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    // Track which path we're fetching to avoid stale updates.
    const requestIdRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(0);
    const setData = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((next, key = "")=>setLoaded(next ? {
            key,
            data: next
        } : null), []);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        // Bump on every run, including the early returns, so an in-flight fetch for the previous file goes stale.
        const requestId = ++requestIdRef.current;
        if (!sessionId || !filePath) {
            setData(null);
            setLoading(false);
            setError(null);
            return;
        }
        // Check module-level cache first – avoids re-fetching on tab re-entry.
        const cacheKey = `${sessionId}:${filePath}`;
        const cached = fileContentCache.get(cacheKey);
        if (cached && Date.now() - cached.timestamp < FILE_CONTENT_CACHE_TTL_MS) {
            setData(cached.data, cacheKey);
            setLoading(false);
            setError(null);
            return;
        }
        setLoading(true);
        setError(null);
        setData(null);
        fetchFileContent(sessionId, filePath, baseUrl).then((response)=>{
            if (requestId === requestIdRef.current) {
                fileContentCache.set(cacheKey, {
                    data: response,
                    timestamp: Date.now()
                });
                setData(response, cacheKey);
            }
        }).catch((err)=>{
            if (requestId === requestIdRef.current) {
                setError(err instanceof Error ? err.message : "Failed to load file content");
                console.error("Error fetching file content:", err);
            }
        }).finally(()=>{
            if (requestId === requestIdRef.current) {
                setLoading(false);
            }
        });
    }, [
        sessionId,
        filePath,
        baseUrl,
        reloadKey,
        setData
    ]);
    const data = loaded?.key === `${sessionId}:${filePath}` ? loaded.data : null;
    return {
        data,
        loading,
        error
    };
}


/***/ }),

/***/ 40920:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Om: () => (/* binding */ createCoalescer),
/* harmony export */   dQ: () => (/* binding */ realClock),
/* harmony export */   hH: () => (/* binding */ COALESCE_MS),
/* harmony export */   iI: () => (/* binding */ createTokenBucket),
/* harmony export */   wT: () => (/* binding */ createWindowAggregator)
/* harmony export */ });
/* unused harmony exports BUCKET_CAPACITY, BUCKET_WINDOW_MS, EXTRA_KEY_WINDOW_MS */
// Volume guards for high-frequency events (ADR-035 section 4). Time is injected so tests use fake clocks.
const realClock = {
    now: ()=>Date.now(),
    setTimeout: (fn, ms)=>setTimeout(fn, ms),
    clearTimeout: (h)=>clearTimeout(h)
};
const BUCKET_CAPACITY = 30;
const BUCKET_WINDOW_MS = 60000;
const EXTRA_KEY_WINDOW_MS = 30000;
/** Events that fire in bursts: only the first per window is kept. */ const COALESCE_MS = {
    "files.tab.switch": 250,
    "files.mobile.keyboard_open": 2000,
    "files.mobile.layout_fallback": 2000
};
/** True when the previous allowed call for `key` is at least `windowMs` old. */ function createCoalescer(clock = realClock) {
    const last = new Map();
    return (key, windowMs)=>{
        const now = clock.now();
        const prev = last.get(key);
        if (prev !== undefined && now - prev < windowMs) return false;
        last.set(key, now);
        return true;
    };
}
/** Per-name fixed-window token bucket; drops are counted and reported once when the window closes. */ function createTokenBucket(onDropped, clock = realClock) {
    const used = new Map();
    const dropped = new Map();
    let timer = null;
    const closeWindow = ()=>{
        timer = null;
        const report = [
            ...dropped
        ];
        used.clear();
        dropped.clear();
        for (const [key, count] of report)onDropped(key, count);
    };
    return (key)=>{
        const n = used.get(key) ?? 0;
        timer ??= clock.setTimeout(closeWindow, BUCKET_WINDOW_MS);
        if (n < BUCKET_CAPACITY) {
            used.set(key, n + 1);
            return true;
        }
        dropped.set(key, (dropped.get(key) ?? 0) + 1);
        return false;
    };
}
/** Counts per key and reports each total once per window (or on flush, e.g. `pagehide`). */ function createWindowAggregator(onClose, clock = realClock) {
    const counts = new Map();
    let timer = null;
    const flush = ()=>{
        if (timer !== null) clock.clearTimeout(timer);
        timer = null;
        const report = [
            ...counts
        ];
        counts.clear();
        for (const [key, count] of report)onClose(key, count);
    };
    const add = (key)=>{
        counts.set(key, (counts.get(key) ?? 0) + 1);
        timer ??= clock.setTimeout(flush, EXTRA_KEY_WINDOW_MS);
    };
    return {
        add,
        flush
    };
}


/***/ }),

/***/ 42467:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  dq: () => (/* binding */ FallbackChip),
  DC: () => (/* binding */ IndexingChip),
  oX: () => (/* binding */ LargeFileStates)
});

// UNUSED EXPORTS: ANNOUNCE_AFTER_MS, INDEXING_TEXT, rangeText, useLoadingAnnouncement

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ./src/lib/files/transport/router.ts
var router = __webpack_require__(88151);
// EXTERNAL MODULE: ./src/components/sessions/files/BannerStack.tsx + 2 modules
var BannerStack = __webpack_require__(77519);
// EXTERNAL MODULE: ./src/components/sessions/files/PendingState.tsx + 1 modules
var PendingState = __webpack_require__(69111);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2Ffiles%2FLargeFileStates.css.ts.vanilla.css%22%2C%22source%22%3A%22Lndvc3JkaTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBmbGV4LXNocmluazogMDsKfQoud29zcmRpMSB7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDZweDsKICBwYWRkaW5nOiAycHggOHB4OwogIGZvbnQtc2l6ZTogMTJweDsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemcpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2Ffiles_2FLargeFileStates_css_ts_vanilla_css_22_2C_22source_22_3A_22Lndvc3JkaTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBmbGV4LXNocmluazogMDsKfQoud29zcmRpMSB7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDZweDsKICBwYWRkaW5nOiAycHggOHB4OwogIGZvbnQtc2l6ZTogMTJweDsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemcpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwp9_22_7D = __webpack_require__(68791);
;// ./src/components/sessions/files/LargeFileStates.css.ts


var chip = 'wosrdi1';
var states = 'wosrdi0';
// EXTERNAL MODULE: ./src/styles/a11y.css.ts
var a11y_css = __webpack_require__(81956);
;// ./src/components/sessions/files/LargeFileStates.tsx
/* __next_internal_client_entry_do_not_use__ ANNOUNCE_AFTER_MS,INDEXING_TEXT,rangeText,FallbackChip,IndexingChip,useLoadingAnnouncement,LargeFileStates auto */ 






/** A loading burst is announced politely once, and only if it outlasts this (ux.md 8.12). */ const ANNOUNCE_AFTER_MS = 500;
const INDEXING_TEXT = "Indexing file...";
const rangeText = (r)=>`${r.startLine.toLocaleString("en-US")}-${r.endLine.toLocaleString("en-US")}`;
const span = (ranges)=>({
        startLine: Math.min(...ranges.map((r)=>r.startLine)),
        endLine: Math.max(...ranges.map((r)=>r.endLine))
    });
/** Quiet chip for a demoted transport: no dialog, no toast. */ function FallbackChip() {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
        className: chip,
        role: "status",
        "data-testid": "fallback-chip",
        children: router/* FALLBACK_CHIP_TEXT */.ML
    });
}
/** The "Indexing file..." chip shown while the server builds the line index. */ function IndexingChip() {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
        className: chip,
        role: "status",
        "data-testid": "indexing-chip",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                "aria-hidden": "true",
                children: "⌛"
            }),
            INDEXING_TEXT
        ]
    });
}
/** Text for the polite live region: "Loading lines A-B..." after 500 ms (once per burst), failures at once. */ function useLoadingAnnouncement(state) {
    const busy = state.loading.length > 0;
    const [burst, setBurst] = (0,react.useState)(null);
    const loadingText = busy ? `Loading lines ${rangeText(span(state.loading))}...` : "";
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!busy) {
            setBurst(null);
            return;
        }
        const id = setTimeout(()=>setBurst((b)=>b ?? loadingText), ANNOUNCE_AFTER_MS);
        return ()=>clearTimeout(id);
    // The text of the first range seen in a burst is the one announced; later ranges in it stay quiet.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        busy
    ]);
    const failed = state.failed.length > 0 ? `Couldn't load lines ${rangeText(span(state.failed))}` : "";
    return failed || burst || "";
}
function specs(file, state, jumpVisible) {
    const out = [];
    const add = (kind, id, body, actions)=>out.push({
            id,
            kind,
            body,
            actions,
            testId: `large-file-${id}`
        });
    for (const range of state.failed){
        add("load_failed", `failed-${range.startLine}`, `Couldn't load lines ${rangeText(range)}.`, [
            {
                label: "Retry",
                onClick: ()=>file.retry(range)
            }
        ]);
    }
    if (state.stale) add("load_failed", "stale", "The file changed on disk. Use Reload from disk to read the new version at this line.");
    if (state.jump !== null && jumpVisible) {
        const body = state.indexing ? `Going to line ${state.jump.toLocaleString("en-US")}... waiting for the file index` : `Going to line ${state.jump.toLocaleString("en-US")}...`;
        add("jump_progress", "jump", body, [
            {
                label: "Cancel",
                onClick: ()=>file.cancelJump()
            }
        ]);
    }
    if (state.notice) add("informational", "notice", state.notice);
    return out;
}
/** Large-file banners (load failure with Retry, far-jump progress with Cancel, clamp notice) and the polite live region. */ function LargeFileStates({ file, state }) {
    const jumpVisible = (0,PendingState/* usePendingDelay */.oQ)(state.jump !== null);
    const announcement = useLoadingAnnouncement(state);
    const banners = (0,react.useMemo)(()=>specs(file, state, jumpVisible).map((b, i)=>({
                ...b,
                seq: i
            })), [
        file,
        state,
        jumpVisible
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: states,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(BannerStack/* BannerStack */.j, {
                banners: banners
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: a11y_css/* visuallyHidden */.Q,
                role: "status",
                "aria-live": "polite",
                "data-testid": "large-file-announce",
                children: announcement
            })
        ]
    });
}


/***/ }),

/***/ 43758:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  _: () => (/* binding */ ConnectionIndicator)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FConnectionIndicator.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7WTwW7qMBBF9_mK2VSChVFCX4GaTf_kydiTZERsR2PnQVPx71UceE3TVqy6s2eO7p3RtV-O-Fqyshig3-5iUa3hLQMo2dt0AIisXCg9Wwnso4q4yA1Wy30GcMkAov-Re9xMyEu2Gg3yxBsKbaNeJZBryKEoGzwPoGqocoIi2iBBo4vIQ7lSrYRNm5DSuygC9SghX22fGO3_6gmpqqOETZ4PtS4gi4AN6ijBeYdD8VRTRBFapXEonli1-4_pijTdiUysJexGw_qqer0ePBtkwcpQFyQ85Q_Jv8GzCDWTO0pI7vMVD43Xx4nV469b3QQkrNszBN-QgX-KF0L8LZ6xX3N_Xk5som-F9o1nOYbZKkYXJxP_SRMflD5W7Dtn5EytW07g7R14ai2h-GHAfiq5uyO5nsLPCb7u85krEvdi0ZCCRctYIgfBaDqNRlgfybvhbYixhU7jMql9Sg5AObJqgIVTFuXtB-1nTdOxGiXz1S7Mu5EsuUqUndMjNCSoeI5RxFFFaN-5OERdkqOIt-91b53x_u0eX_P_LqpLdnkH2YWKby4EAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FConnectionIndicator_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7WTwW7qMBBF9_mK2VSChVFCX4GaTf_kydiTZERsR2PnQVPx71UceE3TVqy6s2eO7p3RtV_O_Fqyshig3_5iUa3hLQMo2dt0AIisXCg9Wwnso4q4yA1Wy30GcMkAov_Re9xMyEu2Gg3yxBsKbaNeJZBryKEoGzwPoGqocoIi2iBBo4vIQ7lSrYRNm5DSuygC9SghX22fGO3_6gmpqqOETZ4PtS4gi4AN6ijBeYdD8VRTRBFapXEonli1_4_pijTdiUysJexGw_qqer0ePBtkwcpQFyQ85Q_Jv8GzCDWTO0pI7vMVD43Xx4nV469b3QQkrNszBN_QgX_KF0L8LZ6xX3N_Xk5som_F9o1nOYbZKkYXJxP_SRMflD5W7Dtn5EytW07g7R14ai2h_GHAfiq5uyO5nsLPCb7u85krEvdi0ZCCRctYIgfBaDqNRlgfybvhbYixhU7jMql9Sg5AObJqgIVTFuXtB_1nTdOxGiXz1S7Mu5EsuUqUndMjNCSoeI5RxFFFaN_5OERdkqOIt_91b53x_u0eX_P_LqpLdnkH2YWKby4EAAA_22_7D = __webpack_require__(43788);
;// ./src/components/backlog/ConnectionIndicator.css.ts


var dots = {live:'z78t1g4 z78t1g1',connecting:'z78t1g3',reconnecting:'z78t1g3',stale:'z78t1g7 z78t1g1',polling:'z78t1g8 z78t1g1'};
var label = 'z78t1g9';
var wrapper = 'z78t1g0';
;// ./src/components/backlog/ConnectionIndicator.tsx
/* __next_internal_client_entry_do_not_use__ ConnectionIndicator auto */ 

// Epic 6.2 (Task 6.2.1b) — small persistent connection-state affordance,
// mounted wherever useWatchBacklogItems is active (ux.md §5, plan.md Domain
// Glossary "ConnectionIndicator"). Deliberately a distinct component from
// web-app/src/components/layout/ConnectionIndicator.tsx: that one is
// WatchSessions-specific (reads sessionsSlice via Redux, offers a
// click-to-reconnect affordance and only has 3 states). This one is a pure
// presentational display driven entirely by the connectionState prop
// useWatchBacklogItems already returns — no store coupling of its own, and
// no reconnect action, since the backlog hook self-heals via its own
// exponential-backoff/backstop timers (plan.md Task 5.4.1a's discovery pass
// confirmed there is no existing session-level indicator to reuse instead).
//
// ux.md §5 UX AC #17 asks for "exactly one of three distinguishable states"
// (connected/reconnecting/degraded) — written before Story 4.2.3 (pre-mortem
// P1 #1: idle-staleness backstop) added a 5th BacklogConnectionState value,
// "stale". Per this epic's task brief, "stale" intentionally gets a visually
// distinct treatment from "reconnecting" rather than being silently folded
// into it: the whole point of the pre-mortem fix was making a
// self-healing-but-currently-not-updating state visible to the user, not
// hiding it behind the same label/animation as an actively-retrying stream.
// "connecting" (the hook's initial pre-first-connect state) shares
// "reconnecting"'s spinner treatment — there is no meaningful difference for
// the user between "hasn't connected yet" and "lost connection, retrying."
const STATE_LABEL = {
    connecting: "Connecting…",
    live: "Live",
    reconnecting: "Reconnecting…",
    stale: "Stale — reconnecting…",
    polling: "Polling (every 30s)"
};
const STATE_ARIA_LABEL = {
    connecting: "Connecting to live backlog updates",
    live: "Live — backlog data is up to date",
    reconnecting: "Reconnecting to live backlog updates",
    stale: "Live backlog updates stalled — reconnecting",
    polling: "Live backlog updates unavailable — refreshing every 30 seconds"
};
function ConnectionIndicator({ connectionState }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: wrapper,
        role: "status",
        "aria-live": "polite",
        "aria-atomic": "true",
        "aria-label": STATE_ARIA_LABEL[connectionState],
        "data-testid": "connection-indicator",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: dots[connectionState],
                "aria-hidden": "true"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: label,
                children: STATE_LABEL[connectionState]
            })
        ]
    });
}


/***/ }),

/***/ 49623:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   e: () => (/* binding */ markdownBody)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(13507);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FmarkdownBody_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA52T0W7CMAxF3_sVfpm0SkuVgmBb_JgpNGkbkcZdmkLHtH_fmgILAca0t0i2r8_9crKqrGRJ4TMBKFCjZbDl9pGQt_xV7md2T9NVAlCicaRTexmX52tf18pIUktV1Y5Bni1XyVeSHaRbL95wWynDgAKNJGb7FOjZANO8c6SolRbBLFmjc9iw82Z_G73xaE4OjghZoOVOoWHQGyHtCBzKqKY6rBrITglXM8gpfRgF1miFtMRyofruwv9z_tPDIG8H6FArEbWVabisQCH9Nh9ryRulP2LhWR8HT7OX54Vs_DpebCqLvRHx2ITTciGUqcaZfCEboNn8OPmrmeU_zLR28nKP6Z6qTP8a9sldVJ1OFbfSlhp3ZGDAe4cx6yn8ENhZbrqWW2lcyGpwvJIwz1Ct109wfKP2modGomXpLvhmni865ou_EG5Yayw27z26A_EUziQ_v5FjE2Z0HWQK6uqfOWOchkfP3zdHlT4nBAAA_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(52298);


var markdownBody = 'gfgef0';

/***/ }),

/***/ 55500:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  Bw: () => (/* binding */ FileTransportProvider),
  J7: () => (/* binding */ useFileTransport)
});

// UNUSED EXPORTS: createDefaultFileTransport

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(4435);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(31381);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(73396);
// EXTERNAL MODULE: ./src/lib/files/analytics/filesAnalytics.ts
var filesAnalytics = __webpack_require__(86832);
;// ./src/lib/files/transport/chunkCache.ts
const MIB = 1024 * 1024;
const CHUNK_CACHE_BUDGET_COARSE = 8 * MIB;
const CHUNK_CACHE_BUDGET_DESKTOP = 64 * MIB;
/** 8 MiB on touch-first devices, 64 MiB elsewhere. */ function chunkBudgetFor(coarsePointer) {
    return coarsePointer ? CHUNK_CACHE_BUDGET_COARSE : CHUNK_CACHE_BUDGET_DESKTOP;
}
const sizeOf = (c)=>Math.max(c.range.endByte - c.range.startByte, c.text.length);
/**
 * LRU-by-bytes store of line windows per file. A chunk is only valid for the validator it was read under:
 * `put` with a different validator empties the file first and reports it.
 */ class ChunkCache {
    constructor(budgetBytes){
        this.budgetBytes = budgetBytes;
        this.files = new Map();
        this.lru = new Map();
        this.used = 0;
    }
    get bytes() {
        return this.used;
    }
    /** The validator the cached chunks of `ref` were read under, if any. */ validatorOf(ref) {
        return this.files.get(keyOf(ref))?.validator;
    }
    /** Stores a chunk; returns true when this replaced the chunks of an older validator. */ put(ref, validator, chunk) {
        const file = keyOf(ref);
        let entry = this.files.get(file);
        const changed = entry !== undefined && entry.validator !== validator;
        if (changed) this.invalidate(ref);
        if (!entry || changed) {
            entry = {
                validator,
                chunks: new Map()
            };
            this.files.set(file, entry);
        }
        const bytes = sizeOf(chunk);
        if (bytes > this.budgetBytes) return changed;
        this.drop(file, chunk.range.startLine);
        entry.chunks.set(chunk.range.startLine, chunk);
        this.lru.set(slotKey(file, chunk.range.startLine), {
            file,
            startLine: chunk.range.startLine,
            bytes
        });
        this.used += bytes;
        while(this.used > this.budgetBytes)this.evictOldest();
        return changed;
    }
    /**
   * Whole chunks that together cover lines [first, last] with no gap, as one result; undefined on any gap.
   * A window never starts or ends mid-line (partial last lines are not stitched across).
   */ get(ref, first, last) {
        const file = keyOf(ref);
        const chunks = this.files.get(file)?.chunks;
        if (!chunks) return undefined;
        const sorted = [
            ...chunks.values()
        ].sort((a, b)=>a.range.startLine - b.range.startLine);
        const picked = [];
        let next = first;
        for (const c of sorted){
            if (c.range.endLine < next) continue;
            if (c.range.startLine > next) return undefined;
            picked.push(c);
            if (c.range.endLine >= last || c.range.eof) {
                for (const p of picked)this.touch(file, p.range.startLine);
                return picked.length === 1 ? c : stitch(picked);
            }
            if (c.partialLastLine) return undefined;
            next = c.range.endLine + 1;
        }
        return undefined;
    }
    invalidate(ref) {
        const file = keyOf(ref);
        const entry = this.files.get(file);
        if (!entry) return;
        for (const startLine of [
            ...entry.chunks.keys()
        ])this.drop(file, startLine);
        this.files.delete(file);
    }
    touch(file, startLine) {
        const k = slotKey(file, startLine);
        const slot = this.lru.get(k);
        if (!slot) return;
        this.lru.delete(k);
        this.lru.set(k, slot);
    }
    drop(file, startLine) {
        const k = slotKey(file, startLine);
        const slot = this.lru.get(k);
        if (!slot) return;
        this.lru.delete(k);
        this.used -= slot.bytes;
        this.files.get(file)?.chunks.delete(startLine);
    }
    evictOldest() {
        const [, slot] = this.lru.entries().next().value;
        this.drop(slot.file, slot.startLine);
    }
}
const keyOf = (ref)=>`${ref.sessionId}\0${ref.path}`;
const slotKey = (file, startLine)=>`${file}\0${startLine}`;
function stitch(chunks) {
    const first = chunks[0];
    const last = chunks[chunks.length - 1];
    return {
        ...last,
        text: chunks.map((c)=>c.text).join(""),
        range: {
            startLine: first.range.startLine,
            endLine: last.range.endLine,
            startByte: first.range.startByte,
            endByte: last.range.endByte,
            bof: first.range.bof,
            eof: last.range.eof
        },
        chunkCrc32c: undefined
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(22800);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var code = __webpack_require__(70792);
// EXTERNAL MODULE: ./src/lib/files/analytics/events.ts
var events = __webpack_require__(80852);
;// ./src/lib/files/transport/crc32c.ts
// CRC32C (Castagnoli, reflected polynomial 0x82F63B78), matching Go's crc32.Castagnoli.
const TABLE = (()=>{
    const t = new Uint32Array(256);
    for(let n = 0; n < 256; n++){
        let c = n;
        for(let k = 0; k < 8; k++)c = c & 1 ? c >>> 1 ^ 0x82f63b78 : c >>> 1;
        t[n] = c >>> 0;
    }
    return t;
})();
/** Pass the previous result as `seed` to checksum data in pieces. */ function crc32c(bytes, seed = 0) {
    let c = ~seed >>> 0;
    for(let i = 0; i < bytes.length; i++)c = TABLE[(c ^ bytes[i]) & 0xff] ^ c >>> 8;
    return ~c >>> 0;
}

// EXTERNAL MODULE: ./src/lib/files/transport/fileTransport.ts
var fileTransport = __webpack_require__(64403);
;// ./src/lib/files/transport/wireMapping.ts
// Proto-to-port mapping shared by the `full` and `connect_unary` adapters.



const READ_ONLY_KINDS = {
    [session_pb/* ReadOnlyReason */.VNo.NONE]: "none",
    [session_pb/* ReadOnlyReason */.VNo.FLAG_OFF]: "flag_off",
    [session_pb/* ReadOnlyReason */.VNo.BINARY]: "binary",
    [session_pb/* ReadOnlyReason */.VNo.TRUNCATED]: "truncated",
    [session_pb/* ReadOnlyReason */.VNo.TOO_LARGE]: "too_large",
    [session_pb/* ReadOnlyReason */.VNo.NON_UTF8]: "non_utf8",
    [session_pb/* ReadOnlyReason */.VNo.MIXED_EOL]: "mixed_eol",
    [session_pb/* ReadOnlyReason */.VNo.SYMLINK]: "symlink",
    [session_pb/* ReadOnlyReason */.VNo.WORKTREE_MISSING]: "worktree_missing",
    [session_pb/* ReadOnlyReason */.VNo.HIDDEN_SESSION]: "hidden_session",
    [session_pb/* ReadOnlyReason */.VNo.GIT_INTERNAL]: "git_internal"
};
const LINE_ENDINGS = {
    [session_pb/* LineEnding */.GkT.LF]: "lf",
    [session_pb/* LineEnding */.GkT.CRLF]: "crlf",
    [session_pb/* LineEnding */.GkT.MIXED]: "mixed"
};
const ANALYSIS = {
    [session_pb/* FileAnalysisState */.kSj.PENDING]: "pending",
    [session_pb/* FileAnalysisState */.kSj.READY]: "ready",
    [session_pb/* FileAnalysisState */.kSj.FAILED]: "failed"
};
const lineEndingOf = (le)=>LINE_ENDINGS[le] ?? "unknown";
const analysisOf = (a)=>ANALYSIS[a] ?? "unspecified";
/** Server reason, falling back to the legacy flags when an older server leaves it unspecified. */ function readOnlyOf(reason, isBinary, truncated) {
    const known = READ_ONLY_KINDS[reason];
    if (known) return known;
    if (isBinary) return "binary";
    return truncated ? "truncated" : "none";
}
// The server sends no typed detail for "index still building"; this is its message (file_ops_range.go lineRangeWindow).
const INDEX_PENDING = /line index/i;
// fetch() rejects with a TypeError worded per browser (Chrome, Firefox, Safari); other TypeErrors are bugs.
const FETCH_FAILURE = /failed to fetch|networkerror|load failed|network request failed|^network error$|^fetch failed$/i;
const isFetchFailure = (e)=>e instanceof TypeError && FETCH_FAILURE.test(e.message);
/** Typed refusals become `Rejected`; wire-level failures become `TransportFailure`; the rest pass through. */ function mapError(err) {
    if (isFetchFailure(err)) return new fileTransport/* TransportFailure */.Az("network", err);
    if (!(err instanceof connect_error/* ConnectError */.T)) return err;
    if (err.code === code/* Code */.C.Unknown && isFetchFailure(err.cause)) return new fileTransport/* TransportFailure */.Az("network", err);
    switch(err.code){
        case code/* Code */.C.NotFound:
            return new fileTransport/* Rejected */.HE("not_found");
        case code/* Code */.C.Unavailable:
            return new fileTransport/* TransportFailure */.Az("unavailable", err);
        case code/* Code */.C.DeadlineExceeded:
            return new fileTransport/* TransportFailure */.Az("deadline", err);
        case code/* Code */.C.ResourceExhausted:
            return err.findDetails(session_pb/* RangeBusyDetailSchema */.hAM).length > 0 ? new fileTransport/* Rejected */.HE("busy") : err;
    }
    const [detail] = err.findDetails(session_pb/* ReadOnlyReasonDetailSchema */.j1A);
    const kind = detail && READ_ONLY_KINDS[detail.reason];
    if (kind && kind !== "none") return new fileTransport/* Rejected */.HE(kind);
    if (err.code === code/* Code */.C.FailedPrecondition) return new fileTransport/* Rejected */.HE(INDEX_PENDING.test(err.rawMessage) ? "index_pending" : "unsupported", err.rawMessage);
    return err;
}
async function guarded(call) {
    try {
        return await call;
    } catch (err) {
        throw mapError(err);
    }
}
async function statVia(client, ref, o, signal) {
    const r = await guarded(client.statFile({
        sessionId: ref.sessionId,
        path: ref.path,
        wantAnalysis: o?.wantAnalysis,
        wantSha256: o?.wantSha256
    }, {
        signal
    }));
    return {
        token: r.versionToken,
        size: Number(r.size),
        readOnly: readOnlyOf(r.readOnlyReason, false, false),
        analysis: analysisOf(r.analysis)
    };
}

;// ./src/lib/files/transport/connectUnaryTransport.ts
// Windowed reads over the unary ReadFileRange RPC (ADR-036): CRC check, torn retry, chunk cache, staleness.






const connectUnaryTransport_MIB = 1024 * 1024;
const MAX_WANT_LINES = 4000;
const UTF8_BOM = [
    0xef,
    0xbb,
    0xbf
];
const CAPABILITIES = {
    protocolVersion: 1,
    features: [],
    ops: [
        "stat",
        "read_all",
        "read_lines",
        "read_bytes"
    ],
    wires: [
        "connect_unary"
    ],
    capsEpoch: 0,
    limits: {
        maxRangeBytes: connectUnaryTransport_MIB,
        maxRangeLines: MAX_WANT_LINES,
        preferredChunkBytes: 128 * 1024,
        maxPatchOps: 0,
        maxPatchBodyBytes: 0,
        maxInsertBytes: 0,
        maxWriteFullBytes: 0,
        hashCapBytes: 64 * connectUnaryTransport_MIB,
        maxConcurrentRanges: 4
    }
};
const DIRS = {
    around: session_pb/* RangeDir */.vrK.AROUND,
    fwd: session_pb/* RangeDir */.vrK.FORWARD,
    back: session_pb/* RangeDir */.vrK.BACKWARD
};
/** 1-based inclusive line span the server would serve for a line request (mirrors firstLineFor). */ function lineSpan(req) {
    const want = Math.min(req.wantLines, MAX_WANT_LINES);
    const raw = req.dir === "fwd" ? req.anchorLine : req.dir === "back" ? req.anchorLine - want + 1 : req.anchorLine - Math.floor(want / 2);
    const first = Math.max(raw, 1);
    return {
        first,
        last: first + want - 1
    };
}
function toRequest(ref, req, ifValidator) {
    const base = {
        sessionId: ref.sessionId,
        path: ref.path,
        ifValidator
    };
    switch(req.kind){
        case "lines":
            return {
                ...base,
                maxBytes: BigInt(req.maxBytes),
                anchor: {
                    case: "lines",
                    value: {
                        anchorLine: BigInt(req.anchorLine),
                        dir: DIRS[req.dir],
                        wantLines: req.wantLines
                    }
                }
            };
        case "bytes":
            return {
                ...base,
                maxBytes: BigInt(req.length),
                anchor: {
                    case: "bytes",
                    value: {
                        offset: BigInt(req.offset),
                        length: BigInt(req.length),
                        dir: DIRS[req.dir ?? "around"]
                    }
                }
            };
        case "all":
            {
                const limit = Math.min(req.maxBytes ?? connectUnaryTransport_MIB, connectUnaryTransport_MIB);
                return {
                    ...base,
                    maxBytes: BigInt(connectUnaryTransport_MIB),
                    preferFullIfLe: BigInt(limit),
                    anchor: {
                        case: "lines",
                        value: {
                            anchorLine: 1n,
                            dir: session_pb/* RangeDir */.vrK.FORWARD,
                            wantLines: MAX_WANT_LINES
                        }
                    }
                };
            }
    }
}
/**
 * The server hashes the raw on-disk bytes but returns LF-normalized, BOM-less, UTF-8-repaired text, so the
 * raw bytes can be rebuilt only for LF or all-CRLF valid UTF-8 windows; others are not checked (null).
 */ function rawBytesOf(r) {
    if (r.isBinary || r.nonUtf8 || r.eolClass === session_pb/* LineEnding */.GkT.MIXED) return null;
    const text = r.eolClass === session_pb/* LineEnding */.GkT.CRLF ? r.content.replace(/\n/g, "\r\n") : r.content;
    const body = new TextEncoder().encode(text);
    if (!r.hasBom) return body;
    const raw = new Uint8Array(UTF8_BOM.length + body.length);
    raw.set(UTF8_BOM);
    raw.set(body, UTF8_BOM.length);
    return raw;
}
function crcMatches(r) {
    const raw = rawBytesOf(r);
    return raw === null || crc32c(raw) === r.chunkCrc32c;
}
function toResult(r) {
    const truncated = !(r.bof && r.eof);
    return {
        text: r.content,
        token: r.versionToken,
        size: Number(r.size),
        lineEnding: lineEndingOf(r.eolClass),
        hasBom: r.hasBom,
        readOnly: readOnlyOf(r.readOnlyReason, r.isBinary, truncated),
        isBinary: r.isBinary,
        truncated,
        range: {
            startLine: Number(r.startLine),
            endLine: Number(r.endLine),
            startByte: Number(r.startByte),
            endByte: Number(r.endByte),
            bof: r.bof,
            eof: r.eof
        },
        capsEpoch: Number(r.capsEpoch),
        chunkCrc32c: r.chunkCrc32c,
        validator: r.validator,
        lineCount: r.lineCountKnown ? Number(r.lineCount) : undefined,
        partialLastLine: r.partialLastLine,
        longLine: r.longLine
    };
}
const isAbort = (err, signal)=>signal?.aborted === true || err instanceof connect_error/* ConnectError */.T && err.code === code/* Code */.C.Canceled || err instanceof DOMException && err.name === "AbortError";
function createConnectUnaryTransport(client, opts) {
    const { floor, cache, analytics } = opts;
    const served = opts.rangeServed ?? (()=>true);
    /** Up to two attempts: a torn window or a CRC mismatch is retried once. */ async function fetchVerified(ref, req, signal) {
        for(let attempt = 0;; attempt++){
            const resp = await guarded(client.readFileRange(toRequest(ref, req, cache?.validatorOf(ref)), {
                signal
            }));
            const ok = !resp.torn && crcMatches(resp);
            if (ok) return {
                resp,
                retried: attempt > 0
            };
            if (attempt === 1) {
                if (resp.torn) {
                    cache?.invalidate(ref);
                    throw new fileTransport/* Rejected */.HE("stale", "the file changed while it was being read");
                }
                throw new fileTransport/* TransportFailure */.Az("malformed");
            }
        }
    }
    /** Detects a validator change against the cached chunks; returns true when the file moved under the reader. */ function noteValidator(ref, validator) {
        const prev = cache?.validatorOf(ref);
        if (prev === undefined || prev === validator) return false;
        cache?.invalidate(ref);
        opts.onChanged?.({
            ref,
            validator
        });
        return true;
    }
    async function readWindow(ref, req, signal) {
        if (req.kind === "lines" && cache) {
            const { first, last } = lineSpan(req);
            const hit = cache.get(ref, first, last);
            if (hit) return {
                result: hit,
                outcome: "ok",
                hit: true
            };
        }
        const { resp, retried } = await fetchVerified(ref, req, signal);
        const result = toResult(resp);
        const changed = noteValidator(ref, resp.validator);
        if (req.kind === "lines" && cache && result.range.endLine >= result.range.startLine && result.range.startLine > 0) {
            cache.put(ref, resp.validator, result);
        }
        return {
            result,
            outcome: changed ? "stale" : retried ? "torn" : "ok",
            hit: false
        };
    }
    return {
        id: "connect_unary",
        capabilities: ()=>CAPABILITIES,
        stat: (ref, o, signal)=>statVia(client, ref, o, signal),
        async read (ref, req, signal) {
            if (!served()) {
                if (req.kind === "all") return floor.read(ref, req, signal);
                throw new fileTransport/* Rejected */.HE("unsupported", `${req.kind} reads are not served`);
            }
            signal?.throwIfAborted();
            const stop = analytics?.time("files.read.window");
            const labels = (outcome, hit, payload)=>({
                    transport: "connect_unary",
                    payload: (0,events/* bucketPayload */.pP)(payload),
                    outcome,
                    cache: hit ? "hit" : "miss"
                });
            try {
                const { result, outcome, hit } = await readWindow(ref, req, signal);
                stop?.(labels(outcome, hit, result.range.endByte - result.range.startByte));
                return result;
            } catch (err) {
                stop?.(labels(isAbort(err, signal) ? "aborted" : err instanceof fileTransport/* Rejected */.HE && err.reason === "stale" ? "stale" : "error", false, 0));
                throw err;
            }
        },
        writeFull: (ref, w, signal)=>floor.writeFull(ref, w, signal)
    };
}

;// ./src/lib/files/transport/fullFileTransport.ts


const fullFileTransport_MIB = 1024 * 1024;
/** Today's behavior: whole-file read over GetFileContent, StatFile, and a 1 MiB editable ceiling. */ const fullFileTransport_CAPABILITIES = {
    protocolVersion: 1,
    features: [],
    ops: [
        "stat",
        "read_all"
    ],
    wires: [
        "connect_unary"
    ],
    capsEpoch: 0,
    limits: {
        maxRangeBytes: 0,
        maxRangeLines: 0,
        preferredChunkBytes: 0,
        maxPatchOps: 0,
        maxPatchBodyBytes: 0,
        maxInsertBytes: 0,
        maxWriteFullBytes: 0,
        hashCapBytes: fullFileTransport_MIB,
        maxConcurrentRanges: 0
    }
};
function createFullFileTransport(client) {
    return {
        id: "full",
        capabilities: ()=>fullFileTransport_CAPABILITIES,
        stat: (ref, o, signal)=>statVia(client, ref, o, signal),
        async read (ref, req, signal) {
            if (req.kind !== "all") throw new fileTransport/* Rejected */.HE("unsupported", `${req.kind} reads need a higher transport`);
            const r = await guarded(client.getFileContent({
                sessionId: ref.sessionId,
                path: ref.path
            }, {
                signal
            }));
            const lines = r.content === "" ? 1 : r.content.split("\n").length;
            return {
                text: r.content,
                token: r.versionToken,
                size: Number(r.size),
                lineEnding: lineEndingOf(r.lineEnding),
                hasBom: r.hasBom,
                readOnly: readOnlyOf(r.readOnlyReason, r.isBinary, r.isTruncated),
                isBinary: r.isBinary,
                truncated: r.isTruncated,
                range: {
                    startLine: 1,
                    endLine: lines,
                    startByte: 0,
                    endByte: Number(r.size),
                    bof: true,
                    eof: !r.isTruncated
                }
            };
        },
        // The WriteFile RPC lands with Story 2.1.2; until then the floor cannot save.
        writeFull: ()=>Promise.reject(new fileTransport/* Rejected */.HE("unsupported", "WriteFile is not available yet"))
    };
}

// EXTERNAL MODULE: ./src/lib/files/transport/router.ts
var transport_router = __webpack_require__(88151);
;// ./src/lib/hooks/useFileTransport.ts
/* __next_internal_client_entry_do_not_use__ createDefaultFileTransport,FileTransportProvider,useFileTransport auto */ 








const FileTransportContext = (0,react.createContext)(null);
const OPS = {
    [session_pb/* FileOp */.kh4.STAT]: "stat",
    [session_pb/* FileOp */.kh4.READ_ALL]: "read_all",
    [session_pb/* FileOp */.kh4.READ_LINES]: "read_lines",
    [session_pb/* FileOp */.kh4.READ_BYTES]: "read_bytes",
    [session_pb/* FileOp */.kh4.READ_STREAM]: "read_stream",
    [session_pb/* FileOp */.kh4.PATCH]: "patch",
    [session_pb/* FileOp */.kh4.WRITE_FULL]: "write_full"
};
const WIRES = {
    [session_pb/* FileWire */.e96.CONNECT_UNARY]: "connect_unary",
    [session_pb/* FileWire */.e96.CONNECT_STREAM]: "connect_stream",
    [session_pb/* FileWire */.e96.WS_STREAM]: "ws_stream",
    [session_pb/* FileWire */.e96.HTTP_RANGE]: "http_range"
};
const defined = (x)=>x !== undefined;
const isCoarsePointer = ()=> false && 0;
/** Production router: the `full` floor, the `connect_unary` range adapter and the capability RPC; stream rungs register as they land. */ function createDefaultFileTransport(analytics) {
    const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    const full = createFullFileTransport(client);
    const unary = createConnectUnaryTransport(client, {
        floor: full,
        cache: new ChunkCache(chunkBudgetFor(isCoarsePointer())),
        // Until the server advertises READ_LINES (files:range-read), `all` reads keep using GetFileContent.
        rangeServed: ()=>router.capabilities().ops.includes("read_lines"),
        analytics
    });
    const router = new transport_router/* FileTransportRouter */.R_({
        full,
        adapters: [
            unary
        ],
        analytics,
        async fetchCapabilities (sessionId) {
            // One cached page-lifetime fetch with no owning component to cancel it.
            // abort-signal-exempt
            const { capabilities: c } = await client.getFileTransportCapabilities({
                sessionId
            });
            const l = c?.limits;
            return {
                protocolVersion: c?.protocolVersion ?? 0,
                features: c?.features ?? [],
                ops: (c?.ops ?? []).map((o)=>OPS[o]).filter(defined),
                wires: (c?.wires ?? []).map((w)=>WIRES[w]).filter(defined),
                capsEpoch: Number(c?.capsEpoch ?? 0),
                limits: {
                    maxRangeBytes: Number(l?.maxRangeBytes ?? 0),
                    maxRangeLines: l?.maxRangeLines ?? 0,
                    preferredChunkBytes: l?.preferredChunkBytes ?? 0,
                    maxPatchOps: l?.maxPatchOps ?? 0,
                    maxPatchBodyBytes: Number(l?.maxPatchBodyBytes ?? 0),
                    maxInsertBytes: Number(l?.maxInsertBytes ?? 0),
                    maxWriteFullBytes: Number(l?.maxWriteFullBytes ?? 0),
                    hashCapBytes: Number(l?.hashCapBytes ?? 0),
                    maxConcurrentRanges: l?.maxConcurrentRanges ?? 0
                }
            };
        }
    });
    return router;
}
/** One router per mounted Files tab: capability cache and demotion state live and die with it. */ function FileTransportProvider({ value, children }) {
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "FileTransport"
    });
    const router = (0,react.useMemo)(()=>value ?? createDefaultFileTransport(analytics), [
        value,
        analytics
    ]);
    return (0,react.createElement)(FileTransportContext.Provider, {
        value: router
    }, children);
}
/** The port every Files hook uses; throws outside a provider so a missing wrapper fails loudly. */ function useFileTransport() {
    const transport = (0,react.useContext)(FileTransportContext);
    if (!transport) throw new Error("useFileTransport must be used within a FileTransportProvider");
    return transport;
}


/***/ }),

/***/ 61991:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O: () => (/* binding */ useWindowedState)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _windowedFile__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(80335);
/* __next_internal_client_entry_do_not_use__ useWindowedState auto */ 

const noop = ()=>()=>undefined;
const empty = ()=>_windowedFile__WEBPACK_IMPORTED_MODULE_1__/* .EMPTY_WINDOWED_STATE */ .W4;
/** Live window state of a windowed file (loads, failures, index, count); the empty state for no file. */ function useWindowedState(file) {
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useSyncExternalStore)(file?.subscribe ?? noop, file?.getSnapshot ?? empty, empty);
}


/***/ }),

/***/ 62921:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  w: () => (/* binding */ InlineNotice)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fcommon%2FInlineNotice.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA62U3W6DMAyF7_sUuVwvMhXarhp7mCklBryBw0zo39R3nwKlggDVJu0GQY45sT87eX4PktNmu9uuxPdCCI1VmatzJJIcTm8LIVSOKUm0UFTtoqysYuukVJWROCh-kvI9eIVLyJfwsnRKqbRGSn11vWrUvWENHImgPInK5Ki9sKIXJVlprKuR066NUfFnyqYm7QcEbUBscsO-1maRGLKywguMvPfLt8V18dyBCRowk07Ba2vVYMkY6TMSK7eSI4HMANPMjuz10D6c5t54amSILRqK3P51QY-xu38iEbjXAkkeUdusSai33Xq-mtUwsc10Yo-3l0d2unsOzLaNWb9fZAj609B9xzVXLrfSIFng2S527B-28aYfpzuRDoe16ZyFk5UaYsOqBV-TBnb9HNYTZeYAPA-ThjBfRvXPz7zPZu4MvCz_wuvrv3F5M3BeTp32PoIesnkQHrfdv89N-CsO9yrDspn6QnGKJHNIbCRUbc3olPvHLtzc_kS6x3Rrj2_ZGLoCPurKYnKWsSELZPuSf-dcfwAzY517yQUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fcommon_2FInlineNotice_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62U3W6DMAyF7_sUuVwvMhXarhp7mCklBryBw0zo39R3nwKlggDVJu0GQY45sT87eX4PktNmu9uuxPdCCI1VmatzJJIcTm8LIVSOKUm0UFTtoqysYuukVJWROCh_kvI9eIVLyJfwsnRKqbRGSn11vWrUvWENHImgPInK5Ki9sKIXJVlprKuR066NUfFnyqYm7QcEbUBscsO_1maRGLKywguMvPfLt8V18dyBCRowk07Ba2vVYMkY6TMSK7eSI4HMANPMjuz10D6c5t54amSILRqK3P51QY_xu38iEbjXAkkeUdusSai33Xq_mtUwsc10Yo_3l0d2unsOzLaNWb9fZAj609B9xzVXLrfSIFng2S527B_28aYfpzuRDoe16ZyFk5UaYsOqBV_TBnb9HNYTZeYAPA_ThjBfRvXPz7zPZu4MvCz_wuvrv3F5M3BeTp32PoIesnkQHrfdv89N_CsO9yrDspn6QnGKJHNIbCRUbc3olPvHLtzc_kS6x3Rrj2_ZGLoCPurKYnKWsSELZPuSf_dcfwAzY517yQUAAA_22_7D = __webpack_require__(77475);
;// ./src/components/common/InlineNotice.css.ts


var actionButton = '_1fx45755';
var actionButtonPrimary = '_1fx45756';
var actions = '_1fx45754';
var body = '_1fx45752';
var container = '_1fx45750';
var dismissButton = '_1fx45757';
var icon = '_1fx45751';
var messageText = '_1fx45753';
;// ./src/components/common/InlineNotice.tsx
/* __next_internal_client_entry_do_not_use__ InlineNotice auto */ 
// +feature: backlog:inline-notice
// InlineNotice — informational (non-error) banner family, added for
// project_plans/backlog-event-driven-updates Epic 5.3 (Task 5.3.2b) and
// reused for Task 5.3.1c's terminal-state banner. Deliberately NOT built on
// InlineError: InlineError is role="alert" aria-live="assertive" (correct
// for real failures), which is the wrong politeness level for a routine,
// non-blocking notice like "this item changed elsewhere" or "this item was
// archived elsewhere" (design/ux.md §6). This component is
// role="status" aria-live="polite" instead, and never blocks interaction
// with the rest of the surrounding form/panel.

function InlineNotice({ message, actions: noticeActions, onDismiss, ...rest }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: container,
        role: "status",
        "aria-live": "polite",
        "data-testid": rest["data-testid"],
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: icon,
                "aria-hidden": "true",
                children: "ⓘ"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: body,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: messageText,
                        children: message
                    }),
                    noticeActions && noticeActions.length > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: actions,
                        children: noticeActions.map((a)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                type: "button",
                                className: a.variant === "primary" ? actionButtonPrimary : actionButton,
                                onClick: a.onClick,
                                children: a.label
                            }, a.label))
                    })
                ]
            }),
            onDismiss && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                className: dismissButton,
                onClick: onDismiss,
                "aria-label": "Dismiss notice",
                children: "\xd7"
            })
        ]
    });
}


/***/ }),

/***/ 64403:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Az: () => (/* binding */ TransportFailure),
/* harmony export */   HE: () => (/* binding */ Rejected),
/* harmony export */   Vc: () => (/* binding */ LADDER)
/* harmony export */ });
// Client port for file I/O (ADR-036). Hooks depend on this, never on the generated Connect client,
// so range reads, deltas and extra wires are adapters behind the router.
/** The server (or the active rung) refused with a reason the UI can state; never triggers demotion. */ class Rejected extends Error {
    constructor(reason, message){
        super(message ?? `rejected: ${reason}`), this.reason = reason;
        this.name = "Rejected";
    }
}
/** Transport-class failure (network, Unavailable, deadline, malformed envelope): counts toward demotion. */ class TransportFailure extends Error {
    constructor(reason, cause){
        super(`transport failure: ${reason}`, {
            cause
        }), this.reason = reason;
        this.name = "TransportFailure";
    }
}
/** Highest rung first. `http_range` is for byte-only consumers and never takes part in the ladder. */ const LADDER = [
    "ws_stream",
    "connect_stream",
    "connect_unary",
    "full"
];


/***/ }),

/***/ 68174:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  w: () => (/* binding */ GuidanceRequestPanel)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ./src/lib/api/connectApi.ts
var connectApi = __webpack_require__(71391);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(4435);
// EXTERNAL MODULE: ./src/gen/session/v1/guidance_request_pb.ts
var guidance_request_pb = __webpack_require__(11127);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(7094);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(73396);
// EXTERNAL MODULE: ./src/lib/api/serialization.ts
var serialization = __webpack_require__(93261);
;// ./src/lib/api/guidanceApi.ts







function getClient() {
    return (0,promise_client/* createClient */.UU)(guidance_request_pb/* GuidanceRequestService */.h$, (0,transport/* getConnectTransport */.qW)());
}
const guidanceApi = connectApi/* connectApi */.X.injectEndpoints({
    endpoints: (builder)=>({
            listGuidanceRequests: builder.query({
                queryFn: async ({ scope, scopeKey })=>{
                    try {
                        const client = getClient();
                        const req = (0,create/* create */.v)(guidance_request_pb/* ListGuidanceRequestsRequestSchema */.vJ, {
                            scope,
                            scopeKey,
                            includeAnswered: true
                        });
                        const response = await client.listGuidanceRequests(req);
                        const requests = response.requests.map((r)=>(0,serialization/* toPlainObject */.V)(r));
                        return {
                            data: {
                                requests,
                                pendingCount: response.pendingCount,
                                cap: response.cap
                            }
                        };
                    } catch (err) {
                        const msg = err instanceof Error ? err.message : "Failed to fetch guidance requests";
                        return {
                            error: {
                                status: -1,
                                error: msg
                            }
                        };
                    }
                },
                providesTags: [
                    "GuidanceRequests"
                ]
            }),
            getGuidanceRequest: builder.query({
                queryFn: async (id)=>{
                    try {
                        const response = await getClient().getGuidanceRequest((0,create/* create */.v)(guidance_request_pb/* GetGuidanceRequestRequestSchema */.g3, {
                            id
                        }));
                        if (!response.request) return {
                            error: {
                                status: -1,
                                error: "Guidance request not found"
                            }
                        };
                        return {
                            data: (0,serialization/* toPlainObject */.V)(response.request)
                        };
                    } catch (err) {
                        const msg = err instanceof Error ? err.message : "Failed to fetch guidance request";
                        return {
                            error: {
                                status: -1,
                                error: msg
                            }
                        };
                    }
                },
                providesTags: (_result, _error, id)=>[
                        {
                            type: "GuidanceRequests",
                            id
                        }
                    ]
            }),
            listSavedGuidanceAnswers: builder.query({
                queryFn: async ()=>{
                    try {
                        const response = await getClient().listSavedGuidanceAnswers((0,create/* create */.v)(guidance_request_pb/* ListSavedGuidanceAnswersRequestSchema */.zR, {}));
                        return {
                            data: response.requests.map((r)=>(0,serialization/* toPlainObject */.V)(r))
                        };
                    } catch (err) {
                        const msg = err instanceof Error ? err.message : "Failed to fetch saved answers";
                        return {
                            error: {
                                status: -1,
                                error: msg
                            }
                        };
                    }
                },
                providesTags: [
                    "GuidanceRequests"
                ]
            }),
            answerGuidanceRequest: builder.mutation({
                queryFn: async ({ id, answer })=>{
                    try {
                        const client = getClient();
                        const req = (0,create/* create */.v)(guidance_request_pb/* AnswerGuidanceRequestRequestSchema */.Ko, {
                            id,
                            answer
                        });
                        // applied is false when a losing racer answers an already-answered/cancelled
                        // request (server-side dedup guard) — callers must surface that, not treat it
                        // as a normal success.
                        const response = await client.answerGuidanceRequest(req);
                        return {
                            data: {
                                applied: response.applied
                            }
                        };
                    } catch (err) {
                        const msg = err instanceof Error ? err.message : "Failed to submit answer";
                        return {
                            error: {
                                status: -1,
                                error: msg
                            }
                        };
                    }
                },
                invalidatesTags: [
                    "GuidanceRequests"
                ]
            })
        })
});
const { useListGuidanceRequestsQuery, useAnswerGuidanceRequestMutation, useGetGuidanceRequestQuery, useListSavedGuidanceAnswersQuery } = guidanceApi;

// EXTERNAL MODULE: ./src/lib/utils/connectError.ts
var connectError = __webpack_require__(22602);
// EXTERNAL MODULE: ./src/lib/guidance/deliveryView.ts
var deliveryView = __webpack_require__(32848);
// EXTERNAL MODULE: ./src/components/guidance/GuidanceDeliveryLine.tsx
var GuidanceDeliveryLine = __webpack_require__(36108);
// EXTERNAL MODULE: ./src/components/guidance/GuidanceQuestionForm.tsx
var GuidanceQuestionForm = __webpack_require__(71592);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fguidance%2FGuidanceRequestPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VVwZKiMBC9-xU5DodYgDs6Ez9mKyQtdhkTNgmibM2_bxEcFgVkrDls7RH6dffLe53O8vCrWgkRk98LQjIuDrk1pZaMnLh9ofRn8g51autNtG3ixkqwjCTFmTijUN6hoIeiCnaekdUEFPtQyyWW7r7n6i1gCi4l6nwQTaLt4mOxbPkngf-R2xw1I_G2-6CZ8d4cB9lxqL0z2lOHNQzi4m-8Asz3npFNHOoKo4y9x8d9MmkgM4pL0pm-WTTLPa37zVahmULnqfMXBYxoo2F7r0anYviS6ArFL4zsFJwDIQVnKtGC8Gg0a8iXR91Ecl6MitcR-NGOTuukN8XceFyZtNCHhdkOrfNU7FHJQZPPU96Ui3vpryFlcFKuMNcUPRwdIxl3oLAtNHLQoPRTbqxD02l7-XCs1vGtQWlxHqfx-LKsA6baowfqCi7CIFSWF529bm9RH64TMH3VL9HklNdf2gLnviCb_1-Q07Qg1ZcEKfuCvA031dwWmt84788UzaaL3uxUPn6BgnSNjox8qjl1dbpSWSg1scnTOhrfzk89NxNTcH24Js2dUXhex7HRFaV1Tb3CoPZgm1_ecu2wXa5cKRIvk1dHgDvoy8T25gSWaeNfmETHMwUymnma4UboLi1kmYIL9BdG4uW6T0wbT7lSpgLZSxbjhs-62_Zq4IwkD57sf2r0-ntGf_wBnWnyDykJAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fguidance_2FGuidanceRequestPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VVwZKiMBC9_xU5DodYgDs6Ez9mKyQtdhkTNgmibM2_bxEcFgVkrDls7RH6dffLe53O8vCrWgkRk98LQjIuDrk1pZaMnLh9ofRn8g51autNtG3ixkqwjCTFmTijUN6hoIeiCnaekdUEFPtQyyWW7r7n6i1gCi4l6nwQTaLt4mOxbPkngf_R2xw1I_G2_6CZ8d4cB9lxqL0z2lOHNQzi4m_8Asz3npFNHOoKo4y9x8d9MmkgM4pL0pm_WTTLPa37zVahmULnqfMXBYxoo2F7r0anYviS6ArFL4zsFJwDIQVnKtGC8Gg0a8iXR91Ecl6MitcR_NGOTuukN8XceFyZtNCHhdkOrfNU7FHJQZPPU96Ui3vpryFlcFKuMNcUPRwdIxl3oLAtNHLQoPRTbqxD02l7_XCs1vGtQWlxHqfx_LKsA6baowfqCi7CIFSWF529bm9RH64TMH3VL9HklNdf2gLnviCb_1_Q07Qg1ZcEKfuCvA031dwWmt84788UzaaL3uxUPn6BgnSNjox8qjl1dbpSWSg1scnTOhrfzk89NxNTcH24Js2dUXhex7HRFaV1Tb3CoPZgm1_ecu2wXa5cKRIvk1dHgDvoy8T25gSWaeNfmETHMwUymnma4UboLi1kmYIL9BdG4uW6T0wbT7lSpgLZSxbjhs_62_Zq4IwkD57sf2r0_ntGf_wBnWnyDykJAAA_22_7D = __webpack_require__(23810);
;// ./src/components/guidance/GuidanceRequestPanel.css.ts


var actions = 'kqw3cca';
var answerButton = 'kqw3ccb';
var answerText = 'kqw3cc9';
var badgeAnswered = 'kqw3cc7';
var badgePending = 'kqw3cc6';
var errorText = 'kqw3cc2';
var heading = 'kqw3cc1';
var item = 'kqw3cc4';
var itemHeader = 'kqw3cc5';
var list = 'kqw3cc3';
var panel = 'kqw3cc0';
var questionText = 'kqw3cc8';
var shortAnswerForm = 'kqw3ccc';
var shortAnswerInput = 'kqw3ccd';
;// ./src/components/guidance/LegacyAnswerForm.tsx
/* __next_internal_client_entry_do_not_use__ LegacyAnswerForm auto */ 


function LegacyAnswerForm(props) {
    switch(props.request.questionType){
        case "yes-no":
            return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(YesNoAnswer, {
                ...props
            });
        case "multiple-choice":
            return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(MultipleChoiceAnswer, {
                ...props
            });
        default:
            return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ShortAnswerForm, {
                ...props
            });
    }
}
function YesNoAnswer({ request, busy, onSubmit }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: actions,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                className: answerButton,
                disabled: busy,
                onClick: ()=>onSubmit("yes"),
                "data-testid": `guidance-answer-yes-${request.id}`,
                children: "Yes"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                className: answerButton,
                disabled: busy,
                onClick: ()=>onSubmit("no"),
                "data-testid": `guidance-answer-no-${request.id}`,
                children: "No"
            })
        ]
    });
}
function MultipleChoiceAnswer({ request, busy, onSubmit }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        className: actions,
        children: request.options.map((opt)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                className: answerButton,
                disabled: busy,
                onClick: ()=>onSubmit(opt),
                "data-testid": `guidance-answer-option-${request.id}-${opt}`,
                children: opt
            }, opt))
    });
}
function ShortAnswerForm({ request, busy, onSubmit }) {
    const [value, setValue] = (0,react.useState)("");
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("form", {
        className: shortAnswerForm,
        onSubmit: (e)=>{
            e.preventDefault();
            const trimmed = value.trim();
            if (trimmed) onSubmit(trimmed);
        },
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                type: "text",
                className: shortAnswerInput,
                value: value,
                onChange: (e)=>setValue(e.target.value),
                placeholder: "Type your answer…",
                disabled: busy,
                "aria-label": `Answer for: ${request.questionText}`,
                "data-testid": `guidance-answer-input-${request.id}`
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "submit",
                className: answerButton,
                disabled: busy || !value.trim(),
                "data-testid": `guidance-answer-submit-${request.id}`,
                children: "Submit"
            })
        ]
    });
}

;// ./src/components/guidance/GuidanceRequestPanel.tsx
/* __next_internal_client_entry_do_not_use__ GuidanceRequestPanel auto */ 
// +feature: guidance:request-panel








/** Encapsulates the answer-submission mutation + in-flight/error state, split out to keep GuidanceRequestPanel short. */ function useGuidanceAnswering() {
    const [answerGuidanceRequest] = useAnswerGuidanceRequestMutation();
    const [answeringId, setAnsweringId] = (0,react.useState)(null);
    const [answerError, setAnswerError] = (0,react.useState)(null);
    const submitAnswer = (0,react.useCallback)(async (id, answer)=>{
        setAnsweringId(id);
        setAnswerError(null);
        try {
            const result = await answerGuidanceRequest({
                id,
                answer
            }).unwrap();
            if (!result.applied) {
                setAnswerError("This question was already answered by someone else.");
            }
        } catch (err) {
            setAnswerError((0,connectError/* getErrorMessage */.u)(err, "Failed to submit answer. It may no longer be pending — reload and try again."));
        } finally{
            setAnsweringId(null);
        }
    }, [
        answerGuidanceRequest
    ]);
    return {
        answeringId,
        answerError,
        submitAnswer
    };
}
/**
 * GuidanceRequestPanel — the one shared component (AC3) rendering durable
 * guidance requests' pending/answered state. Embedded identically in
 * BacklogItemDetail, TriageReviewPanel, and SessionDetailView rather than
 * forked per view, per project_plans/durable-guidance-request's Phase 5 —
 * that duplication would also trip the jscpd/dupl duplication gates
 * (web-app/.jscpd.json, .golangci.yml).
 *
 * Renders nothing (no empty state) when the scope has no open/recent
 * requests, mirroring TriageReviewPanel's "nothing to show" convention.
 */ function GuidanceRequestPanel({ scope, scopeKey, title = "Guidance requests" }) {
    const { data, error, isLoading } = useListGuidanceRequestsQuery({
        scope,
        scopeKey
    }, {
        skip: !scopeKey,
        pollingInterval: 5000
    });
    const { answeringId, answerError, submitAnswer } = useGuidanceAnswering();
    if (!scopeKey || isLoading || error) return null;
    const requests = data?.requests ?? [];
    if (requests.length === 0) return null;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("section", {
        className: panel,
        "aria-live": "polite",
        "data-testid": "guidance-request-panel",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h3", {
                className: heading,
                children: title
            }),
            answerError && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                className: errorText,
                role: "alert",
                children: answerError
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("ul", {
                className: list,
                children: requests.map((r)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)(GuidanceRequestItem, {
                        request: r,
                        busy: answeringId === r.id,
                        onSubmit: (answer)=>void submitAnswer(r.id, answer)
                    }, r.id))
            })
        ]
    });
}
function GuidanceRequestItem({ request, busy, onSubmit }) {
    const isPending = request.status === "pending";
    if ((request.questions?.length ?? 0) > 0) return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(StructuredRequestItem, {
        request: request,
        busy: busy,
        onSubmit: onSubmit
    });
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("li", {
        className: item,
        "data-testid": `guidance-request-${request.id}`,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: itemHeader,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: isPending ? badgePending : badgeAnswered,
                        "data-testid": "guidance-request-status",
                        children: isPending ? "Pending" : "Answered"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: questionText,
                        children: request.questionText
                    })
                ]
            }),
            isPending ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)(LegacyAnswerForm, {
                request: request,
                busy: busy,
                onSubmit: onSubmit
            }) : /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                className: answerText,
                children: [
                    "Answer: ",
                    request.answer
                ]
            })
        ]
    });
}
/** A request that carries structured questions: the shared form, then its delivery-state line. */ function StructuredRequestItem({ request, busy, onSubmit }) {
    const questions = (request.questions ?? []).map((q)=>({
            text: q.question,
            header: q.header || undefined,
            multiSelect: q.multiSelect,
            options: q.options.map((o)=>({
                    label: o.label,
                    description: o.description || undefined
                }))
        }));
    const view = (0,deliveryView/* deliveryView */.sH)((0,deliveryView/* snapshotOf */.eY)(request), {
        sending: busy
    });
    const pending = request.status === "pending";
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("li", {
        className: item,
        "data-testid": `guidance-request-${request.id}`,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: itemHeader,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: pending ? badgePending : badgeAnswered,
                        "data-testid": "guidance-request-status",
                        children: pending ? "Pending" : "Answered"
                    }),
                    !pending && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: questionText,
                        children: request.questionText
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(GuidanceQuestionForm/* GuidanceQuestionForm */.hI, {
                id: request.id,
                questions: questions,
                mode: pending ? "answer" : "readonly",
                canTypeText: true,
                locked: busy || (view?.locked ?? false),
                openControl: null,
                onSubmit: (answers)=>onSubmit((0,GuidanceQuestionForm/* formatAnswers */.BM)(answers)),
                delivery: view && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(GuidanceDeliveryLine/* GuidanceDeliveryLine */.V, {
                    view: view,
                    onAction: ()=>{},
                    openControl: null
                })
            })
        ]
    });
}


/***/ }),

/***/ 69111:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  ls: () => (/* binding */ PENDING_MS),
  Dt: () => (/* binding */ WaitState),
  fI: () => (/* binding */ failureClass),
  v_: () => (/* binding */ reloadFailedBanner),
  oQ: () => (/* binding */ usePendingDelay)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ./src/lib/files/analytics/sampling.ts
var sampling = __webpack_require__(40920);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2Ffiles%2FPendingState.css.ts.vanilla.css%22%2C%22source%22%3A%22QGtleWZyYW1lcyBfMWx1ajdrYzAgewogIHRvIHsKICAgIHRyYW5zZm9ybTogcm90YXRlKDM2MGRlZyk7CiAgfQp9Ci5fMWx1ajdrYzEgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogY2VudGVyOwogIGdhcDogMTBweDsKICBwYWRkaW5nOiAyNHB4IDE2cHg7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiAxNHB4OwogIHRleHQtYWxpZ246IGNlbnRlcjsKfQouXzFsdWo3a2MyIHsKICB3aWR0aDogMjBweDsKICBoZWlnaHQ6IDIwcHg7CiAgYm9yZGVyLXJhZGl1czogNTAlOwogIGJvcmRlcjogMnB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6Zyk7CiAgYm9yZGVyLXRvcC1jb2xvcjogdmFyKC0tXzE5ZXoycnptKTsKICBhbmltYXRpb246IF8xbHVqN2tjMCAwLjhzIGxpbmVhciBpbmZpbml0ZTsKfQouXzFsdWo3a2MzIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxMik7Cn0KLl8xbHVqN2tjNCB7CiAgZGlzcGxheTogZmxleDsKICBnYXA6IDhweDsKICBmbGV4LXdyYXA6IHdyYXA7CiAganVzdGlmeS1jb250ZW50OiBjZW50ZXI7Cn0KLl8xbHVqN2tjNSB7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnpnKTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBiYWNrZ3JvdW5kOiB0cmFuc3BhcmVudDsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKICBmb250OiBpbmhlcml0OwogIHBhZGRpbmc6IDRweCAxMnB4OwogIGN1cnNvcjogcG9pbnRlcjsKfQpAbWVkaWEgKHByZWZlcnMtcmVkdWNlZC1tb3Rpb246IHJlZHVjZSkgewogIC5fMWx1ajdrYzIgewogICAgYW5pbWF0aW9uOiBub25lOwogIH0KfQpAbWVkaWEgKHBvaW50ZXI6IGNvYXJzZSkgewogIC5fMWx1ajdrYzUgewogICAgbWluLWhlaWdodDogNDRweDsKICAgIG1pbi13aWR0aDogNDRweDsKICB9Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2Ffiles_2FPendingState_css_ts_vanilla_css_22_2C_22source_22_3A_22QGtleWZyYW1lcyBfMWx1ajdrYzAgewogIHRvIHsKICAgIHRyYW5zZm9ybTogcm90YXRlKDM2MGRlZyk7CiAgfQp9Ci5fMWx1ajdrYzEgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogY2VudGVyOwogIGdhcDogMTBweDsKICBwYWRkaW5nOiAyNHB4IDE2cHg7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiAxNHB4OwogIHRleHQtYWxpZ246IGNlbnRlcjsKfQouXzFsdWo3a2MyIHsKICB3aWR0aDogMjBweDsKICBoZWlnaHQ6IDIwcHg7CiAgYm9yZGVyLXJhZGl1czogNTAlOwogIGJvcmRlcjogMnB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6Zyk7CiAgYm9yZGVyLXRvcC1jb2xvcjogdmFyKC0tXzE5ZXoycnptKTsKICBhbmltYXRpb246IF8xbHVqN2tjMCAwLjhzIGxpbmVhciBpbmZpbml0ZTsKfQouXzFsdWo3a2MzIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxMik7Cn0KLl8xbHVqN2tjNCB7CiAgZGlzcGxheTogZmxleDsKICBnYXA6IDhweDsKICBmbGV4LXdyYXA6IHdyYXA7CiAganVzdGlmeS1jb250ZW50OiBjZW50ZXI7Cn0KLl8xbHVqN2tjNSB7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnpnKTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBiYWNrZ3JvdW5kOiB0cmFuc3BhcmVudDsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKICBmb250OiBpbmhlcml0OwogIHBhZGRpbmc6IDRweCAxMnB4OwogIGN1cnNvcjogcG9pbnRlcjsKfQpAbWVkaWEgKHByZWZlcnMtcmVkdWNlZC1tb3Rpb246IHJlZHVjZSkgewogIC5fMWx1ajdrYzIgewogICAgYW5pbWF0aW9uOiBub25lOwogIH0KfQpAbWVkaWEgKHBvaW50ZXI6IGNvYXJzZSkgewogIC5fMWx1ajdrYzUgewogICAgbWluLWhlaWdodDogNDRweDsKICAgIG1pbi13aWR0aDogNDRweDsKICB9Cn0_22_7D = __webpack_require__(61095);
;// ./src/components/sessions/files/PendingState.css.ts


var actionButton = '_1luj7kc5';
var actions = '_1luj7kc4';
var failureText = '_1luj7kc3';
var spinner = '_1luj7kc2';
var wait = '_1luj7kc1';
;// ./src/components/sessions/files/PendingState.tsx
/* __next_internal_client_entry_do_not_use__ PENDING_MS,usePendingDelay,failureClass,WaitState,reloadFailedBanner auto */ 



/** Any wait longer than this shows a visible, announced pending state (ux.md 8.5). */ const PENDING_MS = 150;
/** True once `active` has stayed true for `delayMs`; resets the moment it ends. */ function usePendingDelay(active, delayMs = PENDING_MS, clock = sampling/* realClock */.dQ) {
    const [elapsed, setElapsed] = (0,react.useState)(false);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!active) {
            setElapsed(false);
            return;
        }
        const id = clock.setTimeout(()=>setElapsed(true), delayMs);
        return ()=>clock.clearTimeout(id);
    }, [
        active,
        delayMs,
        clock
    ]);
    return active && elapsed;
}
const CLASS_TEXT = {
    not_found: "not found",
    too_large: "too large",
    binary: "binary file",
    encoding: "unsupported encoding",
    worktree_unavailable: "worktree unavailable",
    rpc_error: "network error"
};
/** Words for a failure class; never a path or raw error text. */ const failureClass = (reason)=>CLASS_TEXT[reason];
const PENDING_COPY = {
    open: (c)=>`Opening ${c.name ?? "file"}...`,
    tree: ()=>"Loading files...",
    restore: (c)=>`Restoring ${c.count ?? 0} tabs`,
    outline: ()=>"Reading symbols...",
    stat: ()=>"Checking the file on disk...",
    merge: ()=>"Loading merge view..."
};
const FAILURE_COPY = {
    open: (c, r)=>`Could not open ${c.name ?? "file"}: ${failureClass(r)}`,
    tree: (_c, r)=>`Could not load files: ${failureClass(r)}`,
    restore: (_c, r)=>`Could not restore: ${failureClass(r)}`,
    outline: ()=>"Could not read symbols",
    stat: (_c, r)=>`Could not check the file: ${failureClass(r)}`,
    merge: ()=>"Merge view unavailable - try again"
};
/** The pending or failure panel for one of the waits of ux.md 8.5. */ function WaitState({ kind, name, count, failure }) {
    const ctx = {
        name,
        count
    };
    if (failure) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: wait,
            role: "alert",
            "data-testid": `files-failure-${kind}`,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                    className: failureText,
                    children: FAILURE_COPY[kind](ctx, failure.reason)
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: actions,
                    children: failure.actions.map((a)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            type: "button",
                            className: actionButton,
                            onClick: a.onClick,
                            children: a.label
                        }, a.label))
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: wait,
        role: "status",
        "aria-live": "polite",
        "data-testid": `files-pending-${kind}`,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: spinner,
                "aria-hidden": "true"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                children: PENDING_COPY[kind](ctx)
            })
        ]
    });
}
/** A failed reload never touches the buffer; with unsaved edits it is blocking (priority 1), else priority 8. */ function reloadFailedBanner(f) {
    return {
        id: `reload-failed:${f.path}`,
        kind: f.dirty ? "reload_failed_dirty" : "reload_failed",
        seq: f.seq,
        body: `Could not reload: ${failureClass(f.reason)}`,
        actions: [
            {
                label: "Retry",
                onClick: f.onRetry
            },
            ...f.dirty ? [
                {
                    label: "Copy my version",
                    onClick: f.onCopy
                }
            ] : [],
            {
                label: "Keep editing",
                onClick: f.onKeep
            }
        ]
    };
}


/***/ }),

/***/ 74968:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   R6: () => (/* binding */ decideOpen),
/* harmony export */   Yd: () => (/* binding */ approxLines),
/* harmony export */   rI: () => (/* binding */ lineCountOf)
/* harmony export */ });
/* unused harmony exports T0_MAX_BYTES, classifyTier */
// Tier detection and the open decision for large files (ADR-036 decisions 4-7). Pure: no React, no I/O, no file text.
/** T0 files (at most this many bytes) open whole, exactly as without range reads. */ const T0_MAX_BYTES = 1024 * 1024;
/** T1 is windowed and (later) editable up to the server's hash cap; T2 is windowed and view-only. */ function classifyTier(size, hashCapBytes) {
    if (size <= T0_MAX_BYTES) return "t0";
    return size <= hashCapBytes ? "t1" : "t2";
}
/**
 * Windowed only when range reads are on and served and the file is over 1 MiB text; everything else keeps the
 * base-plan state. Windowed buffers are view-only (`too_large`) until large-file editing lands (Epic 2.x), which
 * is also the reason a T1 file with files:edit-large or files:delta-write off reads the same.
 */ function decideOpen(i) {
    const full = {
        source: {
            kind: "full"
        },
        readOnly: i.readOnly
    };
    if (!i.rangeRead || !i.ops.includes("read_lines")) return full;
    if (i.readOnly === "binary" || i.readOnly === "non_utf8") return full;
    const tier = classifyTier(i.size, i.hashCapBytes);
    if (tier === "t0") return full;
    return {
        source: {
            kind: "windowed",
            tier,
            size: i.size,
            windowStartLine: 1,
            analysis: i.analysis
        },
        readOnly: "too_large"
    };
}
/** The server's exact count when it has one; at end of file the last line number; otherwise size over the window's bytes per line. */ function lineCountOf(size, r) {
    if (r.lineCount !== undefined) return {
        count: r.lineCount,
        exact: true
    };
    const { startLine, endLine, startByte, endByte, eof } = r.range;
    if (eof && !r.partialLastLine) return {
        count: endLine,
        exact: true
    };
    const lines = Math.max(endLine - startLine + 1, 1);
    const perLine = Math.max((endByte - startByte) / lines, 1);
    return {
        count: Math.max(endLine, Math.round(size / perLine)),
        exact: false
    };
}
/** "4.2 million" from a million lines up, thousands separators below. */ function approxLines(n) {
    if (n < 1000000) return n.toLocaleString("en-US");
    return `${(n / 1000000).toFixed(1).replace(/\.0$/, "")} million`;
}


/***/ }),

/***/ 76312:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

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

/***/ 77519:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  j: () => (/* binding */ BannerStack)
});

// UNUSED EXPORTS: BannerItem

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
;// ./src/lib/files/bannerPriority.ts
// Banner kinds and their total order (ux.md 8.2, U-EDIT-15). Later stories register their banners by
// adding a kind here; the stack itself never needs to change.
const BANNER_PRIORITY = {
    save_rejected: 1,
    save_unknown: 1,
    draft_not_stored: 1,
    reload_failed_dirty: 1,
    deleted: 2,
    flag_off: 3,
    stale_dirty: 4,
    offline: 5,
    readonly_reason: 6,
    beyond_loaded: 7,
    stale_clean: 8,
    reload_failed: 8,
    load_failed: 8,
    informational: 9,
    jump_progress: 9,
    indexing: 9,
    fallback: 9
};
const BANNER_KINDS = Object.keys(BANNER_PRIORITY);
/** Priority 1: never collapsed into "+N more", `role="alert"`, no Dismiss. */ const isBlocking = (kind)=>BANNER_PRIORITY[kind] === 1;
/** Highest priority first, then newest. */ function sortBanners(banners) {
    return banners.slice().sort((a, b)=>BANNER_PRIORITY[a.kind] - BANNER_PRIORITY[b.kind] || b.seq - a.seq);
}
/** The first `max` banners, but every blocking one regardless of `max`; the rest are hidden. */ function pickVisible(banners, max) {
    const sorted = sortBanners(banners);
    const blocking = sorted.filter((b)=>isBlocking(b.kind)).length;
    const keep = Math.max(max, blocking);
    return {
        visible: sorted.slice(0, keep),
        hidden: sorted.slice(keep)
    };
}

// EXTERNAL MODULE: ./src/components/sessions/files/StaleBanner.css.ts
var StaleBanner_css = __webpack_require__(32723);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2Ffiles%2FBannerStack.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xM2ZsY3owIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47Cn0KLl8xM2ZsY3oxIHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgZ2FwOiA0cHg7CiAgcGFkZGluZzogMCA4cHg7CiAgZm9udC1zaXplOiAxM3B4OwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6eSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6eik7CiAgYm9yZGVyLWJvdHRvbTogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6eCk7Cn0KLl8xM2ZsY3oyIHsKICBmbGV4OiAxOwogIG1pbi13aWR0aDogMDsKICBvdmVyZmxvdzogaGlkZGVuOwogIHRleHQtb3ZlcmZsb3c6IGVsbGlwc2lzOwogIHdoaXRlLXNwYWNlOiBub3dyYXA7Cn0KLl8xM2ZsY3ozIHsKICBhbGlnbi1zZWxmOiBmbGV4LXN0YXJ0OwogIGJvcmRlcjogbm9uZTsKICBiYWNrZ3JvdW5kOiB0cmFuc3BhcmVudDsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxOSk7CiAgZm9udDogaW5oZXJpdDsKICBmb250LXNpemU6IDEycHg7CiAgdGV4dC1kZWNvcmF0aW9uOiB1bmRlcmxpbmU7CiAgY3Vyc29yOiBwb2ludGVyOwogIHBhZGRpbmc6IDJweCAxMHB4Owp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2Ffiles_2FBannerStack_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xM2ZsY3owIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47Cn0KLl8xM2ZsY3oxIHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgZ2FwOiA0cHg7CiAgcGFkZGluZzogMCA4cHg7CiAgZm9udC1zaXplOiAxM3B4OwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6eSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6eik7CiAgYm9yZGVyLWJvdHRvbTogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6eCk7Cn0KLl8xM2ZsY3oyIHsKICBmbGV4OiAxOwogIG1pbi13aWR0aDogMDsKICBvdmVyZmxvdzogaGlkZGVuOwogIHRleHQtb3ZlcmZsb3c6IGVsbGlwc2lzOwogIHdoaXRlLXNwYWNlOiBub3dyYXA7Cn0KLl8xM2ZsY3ozIHsKICBhbGlnbi1zZWxmOiBmbGV4LXN0YXJ0OwogIGJvcmRlcjogbm9uZTsKICBiYWNrZ3JvdW5kOiB0cmFuc3BhcmVudDsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxOSk7CiAgZm9udDogaW5oZXJpdDsKICBmb250LXNpemU6IDEycHg7CiAgdGV4dC1kZWNvcmF0aW9uOiB1bmRlcmxpbmU7CiAgY3Vyc29yOiBwb2ludGVyOwogIHBhZGRpbmc6IDJweCAxMHB4Owp9_22_7D = __webpack_require__(42934);
;// ./src/components/sessions/files/BannerStack.css.ts


var collapsedRow = '_13flcz1';
var collapsedText = '_13flcz2';
var moreButton = '_13flcz3';
var stack = '_13flcz0';
;// ./src/components/sessions/files/BannerStack.tsx
/* __next_internal_client_entry_do_not_use__ BannerItem,BannerStack auto */ 




const MAX_VISIBLE = 2;
const TARGET = {
    minWidth: 44,
    minHeight: 44
};
/** One banner: `role="alert"` and no Dismiss when blocking, `role="status"` otherwise; it never takes focus. */ function BannerItem({ spec }) {
    const blocking = isBlocking(spec.kind);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: StaleBanner_css/* banner */.v,
        role: blocking ? "alert" : "status",
        "data-testid": spec.testId ?? `banner-${spec.kind}`,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                children: spec.body
            }),
            spec.actions?.map((a)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                    type: "button",
                    className: StaleBanner_css/* bannerAction */.X,
                    onClick: a.onClick,
                    children: a.label
                }, a.label)),
            !blocking && spec.onDismiss && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                className: StaleBanner_css/* bannerAction */.X,
                "aria-label": "Dismiss",
                onClick: spec.onDismiss,
                children: "\xd7"
            })
        ]
    });
}
/** At most two banners by priority; a blocking banner is always shown and "+N more" opens the rest. */ function BannerStack({ banners, collapsed = false }) {
    const [expanded, setExpanded] = (0,react.useState)(false);
    const [opened, setOpened] = (0,react.useState)(false);
    if (banners.length === 0) return null;
    const { visible, hidden } = pickVisible(banners, MAX_VISIBLE);
    const top = visible[0];
    if (collapsed && !opened && isBlocking(top.kind)) {
        const [copy, retry] = top.actions ?? [];
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
            className: stack,
            children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: collapsedRow,
                "data-testid": "banner-collapsed-row",
                style: {
                    minHeight: 44
                },
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        role: "alert",
                        className: collapsedText,
                        children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            type: "button",
                            className: StaleBanner_css/* bannerAction */.X,
                            style: {
                                ...TARGET,
                                textAlign: "left"
                            },
                            onClick: ()=>setOpened(true),
                            children: top.body
                        })
                    }),
                    [
                        copy,
                        retry
                    ].map((a)=>a && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            type: "button",
                            className: StaleBanner_css/* bannerAction */.X,
                            style: TARGET,
                            onClick: a.onClick,
                            children: a.label
                        }, a.label)),
                    banners.length > 1 && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                        type: "button",
                        className: moreButton,
                        style: TARGET,
                        onClick: ()=>setOpened(true),
                        children: [
                            banners.length - 1,
                            " notices"
                        ]
                    })
                ]
            })
        });
    }
    const shown = expanded ? [
        ...visible,
        ...hidden
    ] : visible;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: stack,
        children: [
            shown.map((spec)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)(BannerItem, {
                    spec: spec
                }, spec.id)),
            hidden.length > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                className: moreButton,
                "aria-expanded": expanded,
                onClick: ()=>setExpanded((e)=>!e),
                children: expanded ? "Show fewer" : `+${hidden.length} more`
            })
        ]
    });
}


/***/ }),

/***/ 79472:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   I: () => (/* binding */ getFileIcon)
/* harmony export */ });
/**
 * Returns an emoji/symbol icon for a given filename based on its extension.
 */ function getFileIcon(name) {
    const ext = name.split(".").pop()?.toLowerCase() || "";
    const icons = {
        go: "🐹",
        ts: "𝐓",
        tsx: "⚛",
        js: "𝐉",
        jsx: "⚛",
        py: "🐍",
        rs: "🦀",
        md: "📄",
        json: "{}",
        yaml: "⚙",
        yml: "⚙",
        toml: "⚙",
        sh: "💲",
        css: "🎨",
        html: "🌐"
    };
    return icons[ext] || "📄";
}


/***/ }),

/***/ 80335:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   W4: () => (/* binding */ EMPTY_WINDOWED_STATE),
/* harmony export */   oK: () => (/* binding */ WindowedFile)
/* harmony export */ });
/* unused harmony export WINDOW_LINES */
/* harmony import */ var _lib_files_transport_fileTransport__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(64403);
/* harmony import */ var _windowedSource__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(74968);
// Host side of a windowed (T1/T2) file: reads line windows through the FileTransport port and feeds the lazy
// document of the editor engine (VirtualDoc bridge) or the Accessible view. Holds no file text beyond the call.


/** Lines per window; matches the engine's chunk size so a demanded gap is one request. */ const WINDOW_LINES = 500;
const WINDOW_BYTES = 256 * 1024;
const MAX_CONCURRENT = 2;
const INDEX_POLL_MS = 1500;
const INDEX_POLL_MAX = 80;
const EMPTY_WINDOWED_STATE = {
    lineCount: 1,
    exact: false,
    opened: false,
    indexing: false,
    loading: [],
    failed: [],
    jump: null,
    notice: null,
    demoted: false,
    openFailed: false,
    stale: false
};
const sleep = (ms, signal)=>new Promise((resolve, reject)=>{
        const t = setTimeout(resolve, ms);
        signal.addEventListener("abort", ()=>{
            clearTimeout(t);
            reject(signal.reason);
        }, {
            once: true
        });
    });
/** Settles with `p`, or rejects as soon as `signal` aborts (the shared index wait itself keeps running for other readers). */ const abortable = (p, signal)=>new Promise((resolve, reject)=>{
        if (signal.aborted) return reject(signal.reason);
        signal.addEventListener("abort", ()=>reject(signal.reason), {
            once: true
        });
        p.then(resolve, reject);
    });
const sameRange = (a, b)=>a.startLine === b.startLine && a.endLine === b.endLine;
const aborted = (err)=>err instanceof DOMException && err.name === "AbortError";
class WindowedFile {
    constructor(transport, ref, size){
        this.transport = transport;
        this.ref = ref;
        this.size = size;
        this.state = EMPTY_WINDOWED_STATE;
        this.landing = Promise.resolve();
        this.listeners = new Set();
        this.life = new AbortController();
        this.engine = null;
        this.language = "text";
        this.queue = [];
        this.running = 0;
        this.jumpCtl = null;
        this.indexWait = null;
        this.anchor = null;
        this.attaching = Promise.resolve();
        this.subscribe = (fn)=>{
            this.listeners.add(fn);
            return ()=>this.listeners.delete(fn);
        };
        this.getSnapshot = ()=>this.state;
    }
    set(patch) {
        this.state = {
            ...this.state,
            ...patch
        };
        this.listeners.forEach((fn)=>fn());
    }
    dispose() {
        this.life.abort();
        this.jumpCtl?.abort();
        this.queue = [];
        this.engine = null;
        this.listeners.clear();
    }
    // ---- reads ----
    readOnce(anchorLine, dir, wantLines, signal = this.life.signal) {
        return this.transport.read(this.ref, {
            kind: "lines",
            anchorLine,
            dir,
            wantLines,
            maxBytes: WINDOW_BYTES
        }, signal);
    }
    /** Like `readOnce`, but waits out "line index not ready" (the request is queued behind the index). */ async read(anchorLine, dir, wantLines, signal) {
        for(let attempt = 0;; attempt++){
            try {
                return await this.readOnce(anchorLine, dir, wantLines, signal);
            } catch (err) {
                if (!(err instanceof _lib_files_transport_fileTransport__WEBPACK_IMPORTED_MODULE_0__/* .Rejected */ .HE) || err.reason !== "index_pending" || attempt >= 2) throw err;
                await this.waitForIndex();
            }
        }
    }
    waitForIndex() {
        this.indexWait ??= (async ()=>{
            this.set({
                indexing: true
            });
            try {
                for(let i = 0; i < INDEX_POLL_MAX; i++){
                    await sleep(INDEX_POLL_MS, this.life.signal);
                    const stat = await this.transport.stat(this.ref, {
                        wantAnalysis: true
                    }, this.life.signal);
                    if (stat.analysis !== "pending") return;
                }
            } finally{
                this.indexWait = null;
                if (!this.life.signal.aborted) this.set({
                    indexing: false
                });
            }
        })();
        return this.indexWait;
    }
    /** Lines [first, first + count) as plain strings, for the Accessible view; also settles the line count. */ async pageLines(first, count, signal) {
        const r = await this.read(first, "fwd", count, signal);
        this.noteCount(r);
        const lines = r.text.split("\n");
        if (lines[lines.length - 1] === "") lines.pop();
        return lines.slice(0, count);
    }
    // ---- line count ----
    /** Records the count a result implies; true when the document must be rebuilt for a different count. */ noteCount(r) {
        const next = (0,_windowedSource__WEBPACK_IMPORTED_MODULE_1__/* .lineCountOf */ .rI)(this.size, r);
        if (this.state.exact && next.count === this.state.lineCount) return false;
        if (!next.exact && this.state.exact) return false;
        const changed = next.count !== this.state.lineCount;
        this.set({
            lineCount: next.count,
            exact: next.exact
        });
        return changed && this.engine !== null && this.state.opened;
    }
    // ---- engine bridge ----
    async swapBuffer() {
        const e = this.engine;
        if (!e) return;
        // Marks may change while the buffer builds; rebuild until the buffer carries the current ones.
        let marks;
        let state;
        do {
            marks = this.gutterMarks;
            state = await e.createBufferState({
                doc: "",
                language: this.language,
                gutterMarks: marks,
                lazy: {
                    lineCount: this.state.lineCount
                }
            });
        }while (marks !== this.gutterMarks && this.engine === e);
        if (this.engine === e) e.setBufferState(state);
    }
    /** Hands the engine the text of a window; a window cut mid-line gives up its last line. */ provide(r) {
        if (!this.engine || r.range.endLine < r.range.startLine) return;
        let { endLine } = r.range;
        let text = r.text;
        if (r.partialLastLine && endLine > r.range.startLine) {
            text = text.slice(0, text.lastIndexOf("\n") + 1);
            endLine -= 1;
        }
        this.engine.provideRange({
            startLine: r.range.startLine,
            endLine
        }, text);
    }
    /** Applies new diff marks; once the document is open it is rebuilt around the viewport's top line. */ async setGutterMarks(marks) {
        if (this.gutterMarks === marks) return;
        this.gutterMarks = marks;
        const e = this.engine;
        if (!e || !this.state.opened) return;
        const top = e.getViewport().startLine;
        const anchor = this.anchor;
        try {
            await this.swapBuffer();
            if (this.engine !== e) return;
            const r = await this.read(top, "around", WINDOW_LINES);
            if (this.engine !== e) return;
            this.provide(r);
            // A jump that started meanwhile owns the viewport.
            if (this.jumpCtl === null && this.anchor === anchor) e.gotoLine(top);
        } catch (err) {
            if (!aborted(err)) this.fail(err);
        }
    }
    /** The editor went away (view swap or unmount): stop feeding it. */ detach() {
        this.engine = null;
        this.queue = [];
        this.set({
            opened: false,
            loading: []
        });
    }
    /** Mounts the lazy document on `engine` and paints the window around `anchorLine`; the cursor lands on that exact line. */ attach(engine, language, startLine) {
        this.attaching = this.mount(engine, language, startLine);
        return this.attaching;
    }
    async mount(engine, language, startLine) {
        const anchorLine = this.anchor ?? startLine;
        this.engine = engine;
        this.language = language;
        engine.onNeedRange((range)=>this.request(range));
        let first;
        let deferred = null;
        try {
            try {
                first = await this.readOnce(Math.max(1, anchorLine), "around", WINDOW_LINES);
            } catch (err) {
                // A far line needs the index; line 1 never does, so paint that and jump once the index is ready.
                if (!(err instanceof _lib_files_transport_fileTransport__WEBPACK_IMPORTED_MODULE_0__/* .Rejected */ .HE) || err.reason !== "index_pending") throw err;
                deferred = anchorLine;
                first = await this.readOnce(1, "fwd", WINDOW_LINES);
            }
        } catch (err) {
            return this.fail(err, true);
        }
        if (this.engine !== engine) return;
        const count = (0,_windowedSource__WEBPACK_IMPORTED_MODULE_1__/* .lineCountOf */ .rI)(this.size, first);
        this.set({
            lineCount: count.count,
            exact: count.exact
        });
        await this.swapBuffer();
        if (this.engine !== engine) return;
        this.set({
            opened: true
        });
        this.provide(first);
        const landed = deferred === null ? Math.min(Math.max(1, anchorLine), Math.max(first.range.endLine, 1)) : 1;
        this.anchor = landed;
        engine.gotoLine(landed);
        if (deferred !== null) void this.goTo(deferred);
    }
    fail(err, opening = false) {
        if (aborted(err)) return;
        if (err instanceof _lib_files_transport_fileTransport__WEBPACK_IMPORTED_MODULE_0__/* .Rejected */ .HE && err.reason === "unsupported") this.set({
            demoted: true
        });
        else if (err instanceof _lib_files_transport_fileTransport__WEBPACK_IMPORTED_MODULE_0__/* .Rejected */ .HE && err.reason === "stale") this.set({
            stale: true
        });
        else if (opening) this.set({
            openFailed: true
        });
    }
    // ---- demand loading ----
    request(range) {
        const known = [
            ...this.queue,
            ...this.state.loading
        ].some((r)=>sameRange(r, range));
        if (known) return;
        this.queue.push(range);
        this.pump();
    }
    retry(range) {
        this.set({
            failed: this.state.failed.filter((r)=>!sameRange(r, range))
        });
        this.request(range);
    }
    pump() {
        while(this.running < MAX_CONCURRENT && this.queue.length > 0){
            const range = this.queue.pop(); // newest demand first: the viewport has moved on from older ones
            this.running++;
            this.set({
                loading: [
                    ...this.state.loading,
                    range
                ]
            });
            void this.fill(range).finally(()=>{
                this.running--;
                this.set({
                    loading: this.state.loading.filter((r)=>!sameRange(r, range))
                });
                this.pump();
            });
        }
    }
    async fill(range) {
        try {
            let next = range.startLine;
            while(next <= range.endLine && !this.life.signal.aborted){
                const r = await this.read(next, "fwd", range.endLine - next + 1);
                if (await this.settleCount(r)) return;
                this.provide(r);
                if (r.range.eof || r.range.endLine < next) return;
                next = r.range.endLine + 1;
            }
        } catch (err) {
            if (aborted(err)) return;
            this.fail(err);
            if (!this.state.demoted) this.set({
                failed: [
                    ...this.state.failed,
                    range
                ]
            });
        }
    }
    /** Rebuilds the document for a newly learned count, keeping the viewport's top line; true when it did. */ async settleCount(r) {
        const e = this.engine;
        if (!e || !this.noteCount(r)) return false;
        const top = e.getViewport().startLine;
        await this.swapBuffer();
        if (this.engine !== e) return true;
        const again = await this.read(top, "around", WINDOW_LINES);
        if (this.engine !== e) return true;
        this.provide(again);
        e.gotoLine(top);
        return true;
    }
    // ---- far jumps ----
    /** Loads the window around `requested` first, then puts the cursor on that exact line (clamped to the end). */ goTo(requested) {
        const done = this.jump(requested);
        this.landing = done;
        return done;
    }
    /** Resolves once the latest far jump has landed (or been cancelled); a hit highlight waits for it. */ settled() {
        return this.landing;
    }
    async jump(requested) {
        await this.attaching;
        const e = this.engine;
        let line = Math.max(1, Math.trunc(requested));
        let clamped = false;
        if (this.state.exact && line > this.state.lineCount) {
            line = this.state.lineCount;
            clamped = true;
        }
        this.jumpCtl?.abort();
        const ctl = this.jumpCtl = new AbortController();
        this.life.signal.addEventListener("abort", ()=>ctl.abort(), {
            once: true
        });
        this.set({
            jump: line,
            notice: null
        });
        try {
            const r = await abortable(this.read(line, "around", WINDOW_LINES, ctl.signal), ctl.signal);
            if (ctl.signal.aborted) return {
                line,
                clamped,
                cancelled: true
            };
            if (line > r.range.endLine && r.range.eof) {
                line = r.range.endLine;
                clamped = true;
            }
            const recount = this.noteCount(r);
            if (e && this.engine === e) {
                if (recount) await this.swapBuffer();
                this.provide(r);
                e.gotoLine(line);
            }
            this.anchor = line;
            if (clamped) this.set({
                notice: `Line ${requested.toLocaleString("en-US")} is past the end; showing line ${line.toLocaleString("en-US")}`
            });
            return {
                line,
                clamped,
                cancelled: false
            };
        } catch (err) {
            if (!aborted(err)) this.fail(err);
            return {
                line,
                clamped,
                cancelled: true
            };
        } finally{
            if (this.jumpCtl === ctl) {
                this.jumpCtl = null;
                this.set({
                    jump: null
                });
            }
        }
    }
    cancelJump() {
        this.jumpCtl?.abort();
    }
}


/***/ }),

/***/ 80852:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   D$: () => (/* binding */ bucketKeyCount),
/* harmony export */   Du: () => (/* binding */ bucketResults),
/* harmony export */   FP: () => (/* binding */ bucketTabs),
/* harmony export */   KG: () => (/* binding */ extClass),
/* harmony export */   Ow: () => (/* binding */ bucketSymbols),
/* harmony export */   QO: () => (/* binding */ bucketFiles),
/* harmony export */   Zc: () => (/* binding */ bucketRank),
/* harmony export */   gi: () => (/* binding */ bucketBytes),
/* harmony export */   nO: () => (/* binding */ searchOpts),
/* harmony export */   pP: () => (/* binding */ bucketPayload),
/* harmony export */   sJ: () => (/* binding */ FILES_EVENTS),
/* harmony export */   tF: () => (/* binding */ slugOf),
/* harmony export */   v4: () => (/* binding */ bucketScale),
/* harmony export */   yL: () => (/* binding */ bucketInteractions),
/* harmony export */   yo: () => (/* binding */ bucketQueryLen)
/* harmony export */ });
/* unused harmony exports STAMP_KEYS, bucketHunks, bucketConflicts, bucketOps */
// Single source of truth for Files analytics (ADR-035). The ADR tables are checked against this
// registry by __tests__/events.test.ts, so edit both together.
const BOOL = [
    "true",
    "false"
];
const SIZE = [
    "lt10k",
    "10k-100k",
    "100k-1m",
    "gt1m"
];
const TABS = [
    "0",
    "1",
    "2-3",
    "4-9",
    "10+"
];
const RESULTS = [
    "0",
    "1-9",
    "10-99",
    "100-999",
    "1000+"
];
const FILES = [
    "0",
    "1-9",
    "10-99",
    "100+"
];
const SCALE = [
    "lt1k",
    "1k-10k",
    "10k-50k",
    "50k+"
];
const CONFLICTS = [
    "0",
    "1",
    "2-5",
    "6+"
];
const EXT_CLASS = [
    "code",
    "markup",
    "config",
    "doc",
    "data",
    "other",
    "none"
];
const TRANSPORT = [
    "full",
    "connect_unary",
    "connect_stream",
    "ws_stream",
    "http_range"
];
const TIER = [
    "t0",
    "t1",
    "t2"
];
const PAYLOAD = [
    "lt1k",
    "1k-10k",
    "10k-100k",
    "100k-1m",
    "ge1m"
];
const OPS = [
    "1",
    "2-9",
    "10-99",
    "100-999",
    "1000+"
];
const OP = [
    "read",
    "patch",
    "stat",
    "write_full"
];
const SAVE_REASON = [
    "flag_off",
    "truncated",
    "binary",
    "too_large",
    "non_utf8",
    "mixed_eol",
    "symlink",
    "worktree_missing",
    "hidden_session",
    "git_internal",
    "permission",
    "other"
];
const SERVER_OUTCOME = [
    "ok",
    "truncated",
    "cancelled",
    "invalid",
    "error",
    "disabled",
    "busy"
];
const BIT = [
    "0",
    "1"
];
const SEARCH_OPTS = BIT.flatMap((r)=>BIT.flatMap((c)=>BIT.flatMap((w)=>BIT.flatMap((g)=>BIT.flatMap((i)=>BIT.map((l)=>`r${r}c${c}w${w}g${g}i${i}l${l}`))))));
const ev = (category, labels)=>({
        category,
        labels
    });
const BASE_EVENTS = {
    "files.search.run": ev("user_action", {
        query_len: [
            "2-3",
            "4-8",
            "9-32",
            "33+"
        ],
        opts: SEARCH_OPTS
    }),
    "files.search.results": ev("user_action", {
        outcome: [
            "ok",
            "empty",
            "truncated",
            "invalid_regex",
            "error",
            "worktree_unavailable",
            "flag_off",
            "busy"
        ],
        results: RESULTS,
        files: FILES,
        queued: BOOL
    }),
    "files.search.open_hit": ev("user_action", {
        rank: [
            "1",
            "2-5",
            "6-20",
            "21+"
        ],
        interactions: [
            "1",
            "2",
            "3",
            "4",
            "5+"
        ],
        via: [
            "tap",
            "enter",
            "quickfix"
        ],
        reused_tab: BOOL
    }),
    "files.search.cancel": ev("user_action", {
        phase: [
            "queued",
            "running"
        ],
        results: RESULTS
    }),
    "files.search.queued": ev("user_action", {
        ahead: [
            "1",
            "2+"
        ]
    }),
    "files.tab.open": ev("user_action", {
        kind: [
            "preview",
            "pinned"
        ],
        source: [
            "tree",
            "search",
            "palette",
            "recent",
            "deeplink",
            "outline",
            "jumplist",
            "restore"
        ],
        ext_class: EXT_CLASS,
        size: SIZE
    }),
    "files.tab.close": ev("user_action", {
        dirty: BOOL,
        method: [
            "button",
            "keyboard",
            "context_menu",
            "close_others",
            "evict"
        ],
        open_tabs: TABS
    }),
    "files.tab.switch": ev("navigation", {
        via: [
            "strip",
            "sheet",
            "jumplist",
            "keyboard",
            "quickfix"
        ]
    }),
    "files.tab.evict": ev("user_action", {
        outcome: [
            "evicted_clean_preview",
            "refused_pinned_dirty"
        ]
    }),
    "files.tab.evict_undo": ev("user_action", {}),
    "files.tab.restore": ev("navigation", {
        outcome: [
            "ok",
            "partial",
            "storage_unavailable"
        ],
        tabs: TABS
    }),
    "files.open.failed": ev("user_action", {
        reason: [
            "not_found",
            "too_large",
            "binary",
            "encoding",
            "worktree_unavailable",
            "rpc_error"
        ]
    }),
    "files.nav.back": ev("navigation", {
        via: [
            "button",
            "keyboard",
            "long_press_menu"
        ],
        depth: [
            "1",
            "2-5",
            "6+"
        ]
    }),
    "files.nav.forward": ev("navigation", {
        via: [
            "button",
            "keyboard",
            "long_press_menu"
        ],
        depth: [
            "1",
            "2-5",
            "6+"
        ]
    }),
    "files.nav.jump_line": ev("navigation", {
        via: [
            "palette",
            "status_bar",
            "vim"
        ],
        clamped: BOOL
    }),
    "files.nav.outline": ev("navigation", {
        outcome: [
            "jumped",
            "empty",
            "unavailable_large",
            "closed"
        ],
        symbols: FILES
    }),
    "files.nav.palette": ev("user_action", {
        mode: [
            "file",
            "command",
            "line",
            "outline"
        ],
        outcome: [
            "run",
            "empty",
            "cancel"
        ],
        command: [
            "none",
            "toggle_vim",
            "toggle_wrap",
            "save",
            "reload",
            "shortcuts",
            "goto_line",
            "toggle_accessible_view"
        ]
    }),
    "files.edit.enter": ev("user_action", {
        via: [
            "button",
            "fab",
            "key",
            "vim_i",
            "read_hint"
        ],
        ext_class: EXT_CLASS
    }),
    "files.edit.exit": ev("user_action", {
        via: [
            "stop_editing",
            "flag_off",
            "tab_close"
        ],
        dirty: BOOL
    }),
    "files.edit.read_hint": ev("user_action", {
        outcome: [
            "shown",
            "entered_edit",
            "dismissed"
        ]
    }),
    "files.edit.save": ev("user_action", {
        outcome: [
            "saved",
            "conflict",
            "rejected",
            "unknown",
            "offline_blocked"
        ],
        via: [
            "button",
            "shortcut",
            "vim_w",
            "key_bar",
            "top_bar",
            "palette",
            "save_all"
        ],
        size: SIZE
    }),
    "files.edit.save_conflict": ev("user_action", {
        theirs: [
            "full_text",
            "deleted",
            "too_large",
            "binary",
            "non_utf8",
            "mixed_eol",
            "symlink"
        ],
        merge_available: BOOL
    }),
    "files.edit.save_rejected": ev("user_action", {
        reason: SAVE_REASON
    }),
    "files.edit.merge_open": ev("user_action", {
        hunks: [
            "0",
            "1-3",
            "4-10",
            "11+"
        ],
        conflicts: CONFLICTS
    }),
    "files.edit.merge_saved": ev("user_action", {
        reviewed: BOOL,
        conflicts_resolved: CONFLICTS,
        auto_only: BOOL
    }),
    "files.edit.overwrite": ev("user_action", {
        outcome: [
            "confirmed",
            "cancelled"
        ]
    }),
    "files.edit.reload": ev("user_action", {
        source: [
            "conflict_dialog",
            "stale_banner"
        ],
        copied_first: BOOL
    }),
    "files.edit.draft_restore": ev("user_action", {
        outcome: [
            "restored",
            "restored_conflict",
            "discarded",
            "not_now",
            "missing_file"
        ],
        age: [
            "lt1h",
            "1-6h",
            "6-24h"
        ]
    }),
    "files.edit.draft_not_stored": ev("user_action", {
        reason: [
            "too_large",
            "storage_full",
            "backup_unavailable"
        ]
    }),
    "files.stale.shown": ev("navigation", {
        dirty: BOOL,
        actor: [
            "agent_likely",
            "unknown"
        ]
    }),
    "files.stale.action": ev("user_action", {
        action: [
            "reload",
            "review",
            "dismiss"
        ]
    }),
    "files.vim.toggle": ev("user_action", {
        to: [
            "on",
            "off"
        ],
        source: [
            "status_bar",
            "overflow",
            "palette",
            "auto_hardware"
        ]
    }),
    "files.vim.escape": ev("user_action", {
        via: [
            "esc_esc",
            "status_control",
            "ex_leave"
        ],
        mode: [
            "normal",
            "insert",
            "visual",
            "command"
        ]
    }),
    "files.a11y.accessible_view": ev("user_action", {
        to: [
            "on",
            "off"
        ],
        via: [
            "overflow",
            "palette",
            "skip_link",
            "toolbar"
        ],
        size: SIZE
    }),
    "files.input.profile_change": ev("navigation", {
        to: [
            "touch",
            "hardware"
        ],
        source: [
            "override",
            "auto_pointer",
            "auto_chord",
            "auto_key",
            "auto_viewport"
        ]
    }),
    "files.mobile.layout_fallback": ev("navigation", {
        step: [
            "vim_row_merged",
            "banner_folded",
            "top_bar_hidden",
            "status_hidden",
            "floor"
        ],
        viewport: [
            "xs_portrait",
            "s_portrait",
            "landscape"
        ]
    }),
    "files.mobile.pane_switch": ev("navigation", {
        from: [
            "files",
            "editor",
            "search"
        ],
        to: [
            "files",
            "editor",
            "search"
        ],
        via: [
            "bottom_bar",
            "back_arrow",
            "system_back",
            "tap_hit"
        ]
    }),
    "files.mobile.extra_key_used": ev("user_action", {
        key_class: [
            "esc",
            "tab",
            "modifier",
            "arrow",
            "bracket",
            "punct",
            "undo_redo",
            "save",
            "vim_row"
        ],
        count: [
            "1",
            "2-5",
            "6-20",
            "21+"
        ]
    }),
    "files.mobile.keyboard_open": ev("user_action", {
        trigger: [
            "edit_enter",
            "tap"
        ],
        bar_visible: BOOL
    }),
    "files.perf.open_ms": ev("performance", {
        size: SIZE,
        degraded: BOOL,
        tier: TIER
    }),
    "files.perf.search_first_result_ms": ev("performance", {
        scale: SCALE,
        queued: BOOL
    }),
    "files.perf.search_total_ms": ev("performance", {
        outcome: [
            "ok",
            "truncated",
            "cancelled",
            "error"
        ],
        scale: SCALE
    }),
    "files.perf.save_ms": ev("performance", {
        outcome: [
            "saved",
            "conflict",
            "rejected"
        ],
        size: SIZE
    }),
    "files.transport.select": ev("navigation", {
        op: OP,
        transport: TRANSPORT,
        reason: [
            "capabilities",
            "flag",
            "pinned_full",
            "version"
        ]
    }),
    "files.transport.fallback": ev("performance", {
        op: OP,
        from: TRANSPORT,
        to: TRANSPORT,
        fallback_reason: [
            "network",
            "unavailable",
            "deadline",
            "malformed",
            "capability",
            "version"
        ]
    }),
    "files.read.window": ev("performance", {
        transport: TRANSPORT,
        payload: PAYLOAD,
        outcome: [
            "ok",
            "torn",
            "stale",
            "error",
            "aborted"
        ],
        cache: [
            "hit",
            "miss"
        ]
    }),
    "files.edit.patch_rejected": ev("user_action", {
        patch_reason: [
            "offset_mismatch",
            "result_mismatch",
            "out_of_order",
            "analysis_pending",
            "patch_too_large",
            "diverged"
        ],
        tier: TIER,
        transport: TRANSPORT
    }),
    "files.edit.preflight": ev("user_action", {
        outcome: [
            "ok",
            "lines_moved",
            "mismatch",
            "analysis_pending",
            "error"
        ]
    }),
    "files.server.write": ev("rpc", {
        outcome: [
            "ok",
            "conflict_rejected",
            "denied",
            "disabled",
            "error"
        ],
        reason: [
            "none",
            ...SAVE_REASON
        ],
        size: SIZE,
        eol: [
            "lf",
            "crlf",
            "mixed"
        ],
        bom: BOOL
    }),
    "files.server.search": ev("rpc", {
        outcome: SERVER_OUTCOME,
        results: RESULTS,
        scale: SCALE,
        queued: BOOL,
        backend: [
            "walker"
        ],
        large: BOOL
    }),
    "files.server.search_first_result": ev("performance", {
        scale: SCALE
    }),
    "files.server.patch": ev("rpc", {
        outcome: [
            "ok",
            "conflict",
            "offset_mismatch",
            "result_mismatch",
            "out_of_order",
            "analysis_pending",
            "too_large",
            "rejected",
            "disabled",
            "error"
        ],
        tier: TIER,
        transport: TRANSPORT,
        payload: PAYLOAD,
        ops: OPS
    })
};
function slugOf(name) {
    if (name === "files.perf.search_first_result_ms") return "perf_search_first_ms";
    return name.slice("files.".length).replace(".", "_");
}
const DROPPED_SLUGS = Object.keys(BASE_EVENTS).filter((n)=>!n.startsWith("files.server.")).map((n)=>slugOf(n));
const FILES_EVENTS = {
    ...BASE_EVENTS,
    "files.meta.dropped": ev("performance", {
        event: DROPPED_SLUGS,
        count: [
            "1",
            "2-5",
            "6-20",
            "21+"
        ]
    })
};
/** Own labels the event may carry; the four stamps (form, input, editing, v2) are added by the helper. */ const STAMP_KEYS = (/* unused pure expression or super */ null && ([
    "form",
    "input",
    "editing",
    "v2"
]));
// ---- bucket functions: counts in, closed-enum labels out ----
function bucketer(labels, upper) {
    return (n)=>{
        const i = upper.findIndex((u)=>n <= u);
        return labels[i === -1 ? labels.length - 1 : i];
    };
}
const bucketQueryLen = bucketer([
    "2-3",
    "4-8",
    "9-32",
    "33+"
], [
    3,
    8,
    32
]);
const bucketResults = bucketer(RESULTS, [
    0,
    9,
    99,
    999
]);
const bucketFiles = bucketer(FILES, [
    0,
    9,
    99
]);
const bucketTabs = bucketer(TABS, [
    0,
    1,
    3,
    9
]);
const bucketRank = bucketer([
    "1",
    "2-5",
    "6-20",
    "21+"
], [
    1,
    5,
    20
]);
const bucketInteractions = bucketer([
    "1",
    "2",
    "3",
    "4",
    "5+"
], [
    1,
    2,
    3,
    4
]);
const bucketKeyCount = bucketRank;
const bucketSymbols = bucketFiles;
const bucketHunks = bucketer([
    "0",
    "1-3",
    "4-10",
    "11+"
], [
    0,
    3,
    10
]);
const bucketConflicts = bucketer(CONFLICTS, [
    0,
    1,
    5
]);
const bucketScale = bucketer(SCALE, [
    999,
    9999,
    49999
]);
const bucketBytes = bucketer(SIZE, [
    10239,
    102399,
    1048575
]);
const bucketPayload = bucketer(PAYLOAD, [
    1023,
    10239,
    102399,
    1048575
]);
const bucketOps = bucketer(OPS, [
    1,
    9,
    99,
    999
]);
const EXT_CLASSES = {
    code: [
        "go",
        "ts",
        "tsx",
        "js",
        "jsx",
        "py",
        "rs",
        "java",
        "kt",
        "c",
        "h",
        "cc",
        "cpp",
        "rb",
        "sh",
        "swift",
        "cs",
        "php",
        "css",
        "scss"
    ],
    markup: [
        "html",
        "htm",
        "xml",
        "svg",
        "vue"
    ],
    config: [
        "json",
        "yaml",
        "yml",
        "toml",
        "ini",
        "env",
        "mod",
        "sum",
        "lock",
        "conf"
    ],
    doc: [
        "md",
        "mdx",
        "txt",
        "rst",
        "adoc"
    ],
    data: [
        "csv",
        "tsv",
        "sql",
        "log",
        "ndjson"
    ]
};
/** Returns only the class; the path and extension never leave this function. */ function extClass(path) {
    const base = path.slice(path.lastIndexOf("/") + 1);
    const dot = base.lastIndexOf(".");
    if (dot <= 0 || dot === base.length - 1) return "none";
    const ext = base.slice(dot + 1).toLowerCase();
    for (const [cls, exts] of Object.entries(EXT_CLASSES))if (exts.includes(ext)) return cls;
    return "other";
}
const searchOpts = (o)=>`r${+o.regex}c${+o.caseSensitive}w${+o.wholeWord}g${+o.globs}i${+o.ignored}l${+o.large}`;


/***/ }),

/***/ 86832:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   R: () => (/* binding */ useFilesAnalytics)
/* harmony export */ });
/* unused harmony export createFilesEmitter */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(6000);
/* harmony import */ var _lib_contexts_FeatureFlagsContext__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(79577);
/* harmony import */ var _events__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(80852);
/* harmony import */ var _sampling__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(40920);
/* __next_internal_client_entry_do_not_use__ createFilesEmitter,useFilesAnalytics auto */ // The only code allowed to call `track` with a `files.` name (ADR-035 section 4): typed labels,
// stamped dimensions, sampling, and a guarantee that analytics never throws into UI code.





const MOBILE_MAX_WIDTH = 768;
function readStamps(flags) {
    const hasWindow = "undefined" !== "undefined";
    const coarse = hasWindow && typeof window.matchMedia === "function" && window.matchMedia("(pointer: coarse)").matches;
    return {
        form: hasWindow && window.innerWidth < MOBILE_MAX_WIDTH ? "mobile" : "desktop",
        // Stands in for InputProfile (ADR-033) until that module lands.
        input: coarse ? "touch" : "hardware",
        editing: flags.editing ? "on" : "off",
        v2: flags.v2 ? "on" : "off"
    };
}
function createCore(track, clock) {
    const flagsHolder = {
        flags: {
            editing: false,
            v2: true
        }
    };
    const send = (event, sessionId, component = "FilesTab")=>{
        try {
            track({
                ...event,
                sessionId,
                component,
                labels: {
                    ...event.labels,
                    ...readStamps(flagsHolder.flags)
                }
            });
        } catch  {
        // analytics must never break the viewer
        }
    };
    const allowed = (0,_sampling__WEBPACK_IMPORTED_MODULE_4__/* .createCoalescer */ .Om)(clock);
    const bucket = (0,_sampling__WEBPACK_IMPORTED_MODULE_4__/* .createTokenBucket */ .iI)((slug, count)=>{
        send({
            name: "files.meta.dropped",
            category: "performance",
            labels: {
                event: slug,
                count: (0,_events__WEBPACK_IMPORTED_MODULE_3__/* .bucketKeyCount */ .D$)(count)
            }
        });
    }, clock);
    const keys = (0,_sampling__WEBPACK_IMPORTED_MODULE_4__/* .createWindowAggregator */ .wT)((keyClass, count)=>{
        send({
            name: "files.mobile.extra_key_used",
            category: "user_action",
            labels: {
                key_class: keyClass,
                count: (0,_events__WEBPACK_IMPORTED_MODULE_3__/* .bucketKeyCount */ .D$)(count)
            }
        });
    }, clock);
    const scope = (component, sessionId)=>{
        const emitAt = (name, labels, durationMs)=>{
            try {
                const window_ = _sampling__WEBPACK_IMPORTED_MODULE_4__/* .COALESCE_MS */ .hH[name];
                if (window_ !== undefined && !allowed(name, window_)) return;
                if (!bucket((0,_events__WEBPACK_IMPORTED_MODULE_3__/* .slugOf */ .tF)(name))) return;
                send({
                    name,
                    category: _events__WEBPACK_IMPORTED_MODULE_3__/* .FILES_EVENTS */ .sJ[name].category,
                    durationMs,
                    labels: labels
                }, sessionId, component);
            } catch  {
            // swallowed: see send()
            }
        };
        const emitter = {
            emit: (name, ...args)=>emitAt(name, args[0]),
            emitTimed: (name, durationMs, ...args)=>emitAt(name, args[0], durationMs),
            time: (name)=>{
                const startedAt = clock.now();
                return (...args)=>emitAt(name, args[0], clock.now() - startedAt);
            },
            emitExtraKey: (keyClass)=>keys.add(keyClass),
            flush: ()=>keys.flush()
        };
        return emitter;
    };
    return {
        get flags () {
            return flagsHolder.flags;
        },
        set flags (f){
            flagsHolder.flags = f;
        },
        scope
    };
}
/** Test seam: builds a standalone emitter without React. */ function createFilesEmitter(track, clock = realClock, flags, component = "FilesTab", sessionId) {
    const core = createCore(track, clock);
    core.flags = {
        ...core.flags,
        ...flags
    };
    return core.scope(component, sessionId);
}
const cores = new WeakMap();
const NOOP = {
    emit: ()=>undefined,
    emitTimed: ()=>undefined,
    time: ()=>()=>undefined,
    emitExtraKey: ()=>undefined,
    flush: ()=>undefined
};
function useFilesAnalytics(opts = {}) {
    const ctx = (0,react__WEBPACK_IMPORTED_MODULE_0__.useContext)(_lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_1__/* .AnalyticsContext */ .Ig);
    const { flags } = (0,_lib_contexts_FeatureFlagsContext__WEBPACK_IMPORTED_MODULE_2__/* .useFeatureFlags */ .h)();
    const component = opts.component ?? "FilesTab";
    const core = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>{
        if (!ctx) return null;
        let shared = cores.get(ctx);
        if (!shared) {
            shared = createCore((e)=>ctx.track(e), _sampling__WEBPACK_IMPORTED_MODULE_4__/* .realClock */ .dQ);
            cores.set(ctx, shared);
        }
        return shared;
    }, [
        ctx
    ]);
    // Read at emit time, so a flag flip reaches the next event without a remount.
    if (core) core.flags = {
        editing: flags["files:editing"] ?? false,
        v2: flags["files:viewer-v2"] ?? true
    };
    const emitter = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>core?.scope(component, opts.sessionId) ?? NOOP, [
        core,
        component,
        opts.sessionId
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const onHide = ()=>emitter.flush();
        window.addEventListener("pagehide", onHide);
        return ()=>window.removeEventListener("pagehide", onHide);
    }, [
        emitter
    ]);
    return emitter;
}


/***/ }),

/***/ 88151:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   ML: () => (/* binding */ FALLBACK_CHIP_TEXT),
/* harmony export */   R_: () => (/* binding */ FileTransportRouter)
/* harmony export */ });
/* unused harmony exports DEMOTE_AFTER_FAILURES, DEMOTE_COOLDOWN_MS, SUPPORTED_PROTOCOL_MAJOR */
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(22800);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(70792);
/* harmony import */ var _fileTransport__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(64403);


const DEMOTE_AFTER_FAILURES = 3;
/** A transport-failure demotion lapses after this long, so a network blip never pins a session to a lower rung. */ const DEMOTE_COOLDOWN_MS = 60000;
const SUPPORTED_PROTOCOL_MAJOR = 1;
const FALLBACK_CHIP_TEXT = "Using simpler file mode";
const OPS = [
    "read",
    "patch",
    "stat",
    "write_full"
];
const NEED_FOR_READ = {
    all: "read_all",
    lines: "read_lines",
    bytes: "read_bytes"
};
/** Resolves each op to the highest advertised rung and demotes an op after repeated transport failures. */ class FileTransportRouter {
    constructor(opts){
        this.opts = opts;
        this.caps = null;
        this.capsPromise = null;
        this.pin = null;
        this.capsFailure = null;
        this.byId = new Map();
        this.ceiling = new Map();
        this.streak = new Map();
        this.announced = new Map();
        for (const a of opts.adapters ?? [])this.byId.set(a.id, a);
    }
    capabilities() {
        return this.caps ?? this.opts.full.capabilities();
    }
    stat(ref, o, signal) {
        return this.run(ref, "stat", "stat", (t)=>t.stat(ref, o, signal));
    }
    read(ref, req, signal) {
        return this.run(ref, "read", NEED_FOR_READ[req.kind], (t)=>t.read(ref, req, signal));
    }
    patch(ref, p, signal) {
        return this.run(ref, "patch", "patch", (t)=>t.patch ? t.patch(ref, p, signal) : Promise.reject(new _fileTransport__WEBPACK_IMPORTED_MODULE_0__/* .Rejected */ .HE("unsupported")));
    }
    writeFull(ref, w, signal) {
        return this.run(ref, "write_full", "write_full", (t)=>t.writeFull(ref, w, signal));
    }
    /** The transport an op would use now (after capabilities are loaded). */ transportFor(op) {
        return this.resolve(op, op === "read" ? "read_all" : op).id;
    }
    async run(ref, op, need, call) {
        await this.ensureCaps(ref.sessionId);
        this.requireCapsOrFloor(need);
        const transport = this.resolve(op, need);
        this.announce();
        try {
            const result = await call(transport);
            this.streak.delete(op);
            await this.checkEpoch(ref.sessionId, result.capsEpoch);
            return result;
        } catch (err) {
            this.recordFailure(op, transport, err);
            throw err;
        }
    }
    ensureCaps(sessionId) {
        if (this.caps || this.pin) return Promise.resolve();
        this.capsPromise ??= this.loadCaps(sessionId).finally(()=>{
            this.capsPromise = null;
        });
        return this.capsPromise;
    }
    async loadCaps(sessionId) {
        try {
            const caps = await this.opts.fetchCapabilities(sessionId);
            if (caps.protocolVersion > SUPPORTED_PROTOCOL_MAJOR) this.pin = "version";
            else this.caps = caps;
            this.capsFailure = null;
        } catch (err) {
            // Older server: today's behaviour. Any other failure retries on the next call.
            if (err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .ConnectError */ .T && err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__/* .Code */ .C.Unimplemented) this.pin = "capabilities";
            else this.capsFailure = err;
        }
    }
    /** Unknown capabilities after a failed fetch is transient, so an op only a higher rung serves must not look unsupported. */ requireCapsOrFloor(need) {
        if (this.caps || this.pin || !this.capsFailure) return;
        if (!this.opts.full.capabilities().ops.includes(need)) throw new _fileTransport__WEBPACK_IMPORTED_MODULE_0__/* .TransportFailure */ .Az("unavailable", this.capsFailure);
    }
    async checkEpoch(sessionId, epoch) {
        if (epoch === undefined || !this.caps || epoch === this.caps.capsEpoch) return;
        this.caps = null;
        await this.ensureCaps(sessionId);
        this.announce();
    }
    resolve(op, need) {
        const { full } = this.opts;
        const floor = ()=>{
            if (!full.capabilities().ops.includes(need)) throw new _fileTransport__WEBPACK_IMPORTED_MODULE_0__/* .Rejected */ .HE("unsupported", `${need} needs a higher transport`);
            return full;
        };
        const caps = this.caps;
        if (!caps || this.pin) return floor();
        for (const rung of _fileTransport__WEBPACK_IMPORTED_MODULE_0__/* .LADDER */ .Vc.slice(this.ceilingOf(op))){
            if (rung === "full") break;
            const adapter = this.byId.get(rung);
            if (!adapter || !caps.wires.includes(rung) || !caps.ops.includes(need)) continue;
            if (op === "patch" && !adapter.patch) continue;
            return adapter;
        }
        return floor();
    }
    now() {
        return (this.opts.now ?? Date.now)();
    }
    /** First ladder rung the op may use; a lapsed demotion is forgotten so the higher rung gets another try. */ ceilingOf(op) {
        const c = this.ceiling.get(op);
        if (!c) return 0;
        if (this.now() >= c.until) {
            this.ceiling.delete(op);
            return 0;
        }
        return c.rung;
    }
    selectReason(op) {
        if (this.pin) return this.pin;
        return this.ceilingOf(op) > 0 && this.transportFor(op) === "full" ? "pinned_full" : "capabilities";
    }
    /** One `files.transport.select` per op whenever its resolved transport changes. */ announce() {
        for (const op of OPS){
            let id;
            try {
                id = this.transportFor(op);
            } catch  {
                continue;
            }
            if (this.announced.get(op) === id) continue;
            this.announced.set(op, id);
            this.opts.analytics?.emit("files.transport.select", {
                op,
                transport: id,
                reason: this.selectReason(op)
            });
        }
    }
    recordFailure(op, transport, err) {
        if (!(err instanceof _fileTransport__WEBPACK_IMPORTED_MODULE_0__/* .TransportFailure */ .Az)) {
            this.streak.delete(op); // an application error proves the wire works
            return;
        }
        if (transport.id === "full") return;
        const cur = this.streak.get(op);
        const failures = cur?.id === transport.id ? cur.failures + 1 : 1;
        this.streak.set(op, {
            id: transport.id,
            failures
        });
        if (failures < DEMOTE_AFTER_FAILURES) return;
        this.streak.delete(op);
        this.ceiling.set(op, {
            rung: _fileTransport__WEBPACK_IMPORTED_MODULE_0__/* .LADDER */ .Vc.indexOf(transport.id) + 1,
            until: this.now() + DEMOTE_COOLDOWN_MS
        });
        let to = "full";
        try {
            to = this.transportFor(op);
        } catch  {
        // floor cannot serve this op; keep "full" as the nominal target
        }
        this.opts.analytics?.emit("files.transport.fallback", {
            op,
            from: transport.id,
            to,
            fallback_reason: err.reason
        });
        this.opts.onFallback?.({
            op,
            from: transport.id,
            to,
            text: FALLBACK_CHIP_TEXT
        });
        this.announce();
    }
}


/***/ }),

/***/ 95242:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  hr: () => (/* binding */ FileTree)
});

// UNUSED EXPORTS: TreeRow, buildChangedAncestorPrefixes, buildTreeData, filterChangedTreeData, sortTreeData

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/tree.js + 124 modules
var tree = __webpack_require__(73646);
// EXTERNAL MODULE: ./src/lib/hooks/useFileService.ts
var useFileService = __webpack_require__(37353);
// EXTERNAL MODULE: ./src/lib/utils/fileIcons.ts
var fileIcons = __webpack_require__(79472);
// EXTERNAL MODULE: ./src/lib/utils/truncateMiddle.ts
var truncateMiddle = __webpack_require__(24293);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FFileTree.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7VXXY-bOhB9z6-YPlx1Iy0rIF-tV5X6P6qrygED0xjbtU03ydX-9wqbbAwJ2SS9fYoCY8-Zr3OGrxu2KzStmYHvyernitWzGP6bAFjpfgCspsIUUtcEtLTUsofZMs5ZOX2eALxOXidPh4OJO_GCua0IJHH8T2tRMSwre_wvfzFdcPlCoMI8Z6J9tqbZptSyETmBX1Q_RNH35DPbp3qf5M5NIYWNCloj3w0t0uZoYXDPCCQztX0OcaUOV45GcbojUHC2bY9QjqWI0LLaEMiYsEy3j0uqCHxSzkTRPEdREkiW_kEmudQnEKY9d7Pz7trfKEfNMotSkPaqphYnQJyZsVTb-8AkcR_N3KF5OztXW0hifz7MWeofvVuJtdQ50wQStQUjOeYDMxZYRZrm2BjndByvP5E12rQvlURfiDAEUrVd4wK5AHDTD3zRD_zdCh7yYXecEUBLOWa9C5c3tdGh79OueicBAjSG6cgwzjJLQEjBzqRu2MvLu3KxGrH3zlGKaF0-gi7X9GEeP7az-gjpYvEI8VO6mE7hA9ZKakuF7V37yV0rG8tRMALpSE_ULrudWSSLwjBLIEoHoX2-fUwXPrcDyulqHmlfga4Ap8QT-KbOt5-9SqPYEIjDm7vmsWxrIwcohBLOUXI9T6zffBJIxqjReTy-YJyjMmgctgoti4yiGWvb50VTdXnKAt8ZDGBIRTO0OwLx03xxxTDkIwkLcxG_O3F_h25mA76MPQEF6NkV6JOAJF-6YV7GzqymukQRcVZYArSxcuBuOLRFv7NRuEFYc5lteu3rPR4Vs-PkLgdj49XLgZUqOpvw-lymFn5gqMCaelU6LgHx09JAC5RqQFGgQMuez2QtiLMcSeu4UJ0RoeC-6nZGWA5qv1TbAwF0sa-ltbK-3FIBBhxO6YWepX9VIs_I9VuYs2OYb3TsFSUMhRQya4zXAu9-tFV6x4jiNGOV5HmnPNfQ24_bizcfRDX_0-Jt-jtAkKUzkx4W1i29imom7J_S0AX6u7T0bK4X-is5n99dkFsE7nJUHFCoxn6zO8W-fMwqlm3WcvvxX99VFw7W74tlYC1O0uZ2G7fSpO1yE3erzZmyDUlI_t97pDrdyMeb8j7xxOn9M_PTw5MGvSLQtZG88dyPwq1ujtSvbaQfjbFY7KJMCsuEDV-NR-CZdB-hyF3F-zqjR3TmBNJYE3fK2tPx4Y5gxpku2fdTZi-Qov8c-1qzHCk81HQbdXK_Wq7UdupODj4xAGoU0WENmHf4XyevvwHYeRsOrQ8AAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FFileTree_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VXXY_bOhB9z6_YPlx1Iy0rIF_tV5X6P6qrygED0xjbtU03ydX_9wqbbAwJ2SS9fYoCY8_Zr3OGrxu2KzStmYHvyernitWzGP6bAFjpfgCspsIUUtcEtLTUsofZMs5ZOX2eALxOXidPh4OJO_GCua0IJHH8T2tRMSwre_wvfzFdcPlCoMI8Z6J9tqbZptSyETmBX1Q_RNH35DPbp3qf5M5NIYWNCloj3w0t0uZoYXDPCCQztX0OcaUOV45GcbojUHC2bY9QjqWI0LLaEMiYsEy3j0uqCHxSzkTRPEdREkiW_kEmudQnEKY9d7Pz7trfKEfNMotSkPaqphYnQJyZsVTb_8AkcR_N3KF5OztXW0hifz7MWeofvVuJtdQ50wQStQUjOeYDMxZYRZrm2BjndByvP5E12rQvlURfiDAEUrVd4wK5AHDTD3zRD_zdCh7yYXecEUBLOWa9C5c3tdGh79OueicBAjSG6cgwzjJLQEjBzqRu2MvLu3KxGrH3zlGKaF0_gi7X9GEeP7az_gjpYvEI8VO6mE7hA9ZKakuF7V37yV0rG8tRMALpSE_ULrudWSSLwjBLIEoHoX2_fUwXPrcDyulqHmlfga4Ap8QT_KbOt5_9SqPYEIjDm7vmsWxrIwcohBLOUXI9T6zffBJIxqjReTy_YJyjMmgctgoti4yiGWvb50VTdXnKAt8ZDGBIRTO0OwLx03xxxTDkIwkLcxG_O3F_h25mA76MPQEF6NkV6JOAJF_6YV7GzqymukQRcVZYArSxcuBuOLRFv7NRuEFYc5lteu3rPR4Vs_PkLgdj49XLgZUqOpvw_lymFn5gqMCaelU6LgHx09JAC5RqQFGgQMuez2QtiLMcSeu4UJ0RoeC_6nZGWA5qv1TbAwF0sa_ltbK_3FIBBhxO6YWepX9VIs_I9VuYs2OYb3TsFSUMhRQya4zXAu9_tFV6x4jiNGOV5HmnPNfQ24_bizcfRDX_0_Jt_jtAkKUzkx4W1i29imom7J_S0AX6u7T0bK4X_is5n99dkFsE7nJUHFCoxn6zO8W_fMwqlm3WcvvxX99VFw7W74tlYC1O0uZ2G7fSpO1yE3erzZmyDUlI_t97pDrdyMeb8j7xxOn9M_PTw5MGvSLQtZG88dyPwq1ujtSvbaQfjbFY7KJMCsuEDV_NR_CZdB_hyF3F_zqjR3TmBNJYE3fK2tPx4Y5gxpku2fdTZi_Qov8c_1qzHCk81HQbdXK_Wq7UdupODj4xAGoU0WENmHf4XyevvwHYeRsOrQ8AAA_22_7D = __webpack_require__(55280);
;// ./src/components/sessions/FileTree.css.ts


var container = '_17q7em31';
var empty = '_17q7em35';
var error = '_17q7em33';
var FileTree_css_icon = '_17q7em3a';
var ignored = '_17q7em3c';
var inlineError = '_17q7em3g';
var keyboardFocused = '_17q7em38';
var FileTree_css_lineStats = '_17q7em3r';
var lineStatsAdd = '_17q7em3s';
var lineStatsDel = '_17q7em3t';
var loading = '_17q7em32';
var mark = '_17q7em3n';
var FileTree_css_name = '_17q7em3b';
var FileTree_css_node = '_17q7em36';
var nodeInner = '_17q7em39';
var retryButton = '_17q7em34';
var searchContainer = '_17q7em3h';
var searchEmpty = '_17q7em3o';
var searchInput = '_17q7em3i';
var searchOverlay = '_17q7em3q';
var FileTree_css_searchTruncated = '_17q7em3p';
var selected = '_17q7em37';
var spinner = '_17q7em3f';
var statusBadge = '_17q7em3e';
var symlinkBadge = '_17q7em3d';
var toolbar = '_17q7em3j';
var toolbarButton = '_17q7em3k';
var toolbarLabel = '_17q7em3l';
var treeWrapper = '_17q7em3m';
// EXTERNAL MODULE: ./src/styles/theme.css.ts
var theme_css = __webpack_require__(14925);
// EXTERNAL MODULE: ./src/components/sessions/files/PendingState.tsx + 1 modules
var PendingState = __webpack_require__(69111);
;// ./src/components/sessions/FileTree.tsx
/* __next_internal_client_entry_do_not_use__ buildTreeData,sortTreeData,buildChangedAncestorPrefixes,filterChangedTreeData,TreeRow,FileTree auto */ 








// Git status colors — mapped to theme tokens so they respond to light/dark mode.
const GIT_STATUS_COLORS = {
    M: theme_css.vars.color.gitModified,
    A: theme_css.vars.color.gitAdded,
    D: theme_css.vars.color.gitDeleted,
    "?": theme_css.vars.color.gitUntracked,
    R: theme_css.vars.color.gitRenamed,
    U: theme_css.vars.color.gitConflict
};
// ---- Helpers ----
function fileNodeToTreeNode(fn) {
    return {
        id: fn.path || fn.name,
        name: fn.name,
        isDir: fn.isDir,
        size: fn.size,
        gitStatus: fn.gitStatus,
        isSymlink: fn.isSymlink,
        symlinkTarget: fn.symlinkTarget,
        isIgnored: fn.isIgnored,
        children: fn.isDir ? undefined : undefined
    };
}
/**
 * Build tree data from the directory contents map.
 * Recursively attaches loaded children to each directory node.
 * Exported for unit testing.
 */ function buildTreeData(nodes, dirContents) {
    return nodes.map((node)=>{
        if (!node.isDir) return node;
        const loaded = dirContents.get(node.id);
        if (loaded === undefined) {
            // children: [] makes isLeaf=false (react-arborist checks Array.isArray).
            // This allows node.toggle() to fire, which triggers onToggle → handleToggle → loadDirectory.
            // Children will be populated lazily after the first toggle.
            return {
                ...node,
                children: []
            };
        }
        return {
            ...node,
            children: buildTreeData(loaded, dirContents)
        };
    });
}
function sortKey(node, sortBy) {
    if (sortBy === "name" || node.isDir) return "";
    const dot = node.name.lastIndexOf(".");
    return dot > 0 ? node.name.slice(dot + 1).toLowerCase() : "";
}
/**
 * Sort tree data (dirs always first) by name, or by file extension then name
 * when sortBy is "type". Exported for unit testing.
 */ function sortTreeData(nodes, sortBy) {
    const sorted = [
        ...nodes
    ].sort((a, b)=>{
        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
        const keyDiff = sortKey(a, sortBy).localeCompare(sortKey(b, sortBy));
        if (keyDiff !== 0) return keyDiff;
        return a.name.toLowerCase().localeCompare(b.name.toLowerCase());
    });
    return sorted.map((n)=>n.children ? {
            ...n,
            children: sortTreeData(n.children, sortBy)
        } : n);
}
// ---- Filter: changed files only ----
/**
 * Every ancestor directory path of every changed file, so a "changed only"
 * filter can keep ancestor directories visible even when they haven't been
 * expanded/loaded yet (gitStatusMap covers the whole repo, not just loaded
 * subtrees).
 */ function buildChangedAncestorPrefixes(gitStatusMap) {
    const prefixes = new Set();
    for (const path of gitStatusMap.keys()){
        const parts = path.split("/");
        for(let i = 1; i < parts.length; i++){
            prefixes.add(parts.slice(0, i).join("/"));
        }
    }
    return prefixes;
}
/**
 * Filter tree data down to changed files and their ancestor directories.
 * Operates only on already-loaded/searched nodes — never triggers a fetch.
 * Exported for unit testing.
 */ function filterChangedTreeData(nodes, gitStatusMap, changedAncestorPrefixes) {
    const result = [];
    for (const node of nodes){
        if (node.isDir) {
            if (!changedAncestorPrefixes.has(node.id)) continue;
            const children = node.children ? filterChangedTreeData(node.children, gitStatusMap, changedAncestorPrefixes) : node.children;
            result.push({
                ...node,
                children
            });
        } else if (gitStatusMap.has(node.id)) {
            result.push(node);
        }
    }
    return result;
}
/**
 * Compute which directories have any git-modified descendants.
 */ function computeDirStatuses(nodes, gitStatusMap, result) {
    let anyStatus = false;
    for (const node of nodes){
        if (!node.isDir) {
            const status = gitStatusMap.get(node.id);
            if (status) {
                anyStatus = true;
            }
        } else if (node.children) {
            const childHas = computeDirStatuses(node.children, gitStatusMap, result);
            if (childHas) {
                result.set(node.id, "●");
                anyStatus = true;
            }
        }
    }
    return anyStatus;
}
/**
 * Build a nested TreeNode tree from a flat list of FileNode search results.
 * Ancestor directories are synthesised from file path segments.
 */ function buildSearchTree(files) {
    const nodeMap = new Map();
    for (const file of files){
        const filePath = file.path || file.name;
        const parts = filePath.split("/");
        // Create ancestor directory nodes for each path segment.
        for(let i = 1; i < parts.length; i++){
            const dirPath = parts.slice(0, i).join("/");
            if (!nodeMap.has(dirPath)) {
                nodeMap.set(dirPath, {
                    id: dirPath,
                    name: parts[i - 1],
                    isDir: true,
                    size: BigInt(0),
                    gitStatus: "",
                    isSymlink: false,
                    symlinkTarget: "",
                    isIgnored: false,
                    children: []
                });
            }
        }
        // Create the file node (reuse existing converter for consistency).
        nodeMap.set(filePath, fileNodeToTreeNode(file));
    }
    // Wire children into parents.
    const roots = [];
    for (const [path, node] of nodeMap){
        const lastSlash = path.lastIndexOf("/");
        if (lastSlash === -1) {
            roots.push(node);
        } else {
            const parentPath = path.slice(0, lastSlash);
            const parent = nodeMap.get(parentPath);
            if (parent?.children) {
                parent.children.push(node);
            }
        }
    }
    // Sort recursively: directories first, then alphabetical.
    function sortNodes(nodes) {
        nodes.sort((a, b)=>{
            if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
            return a.name.toLowerCase().localeCompare(b.name.toLowerCase());
        });
        for (const node of nodes){
            if (node.children) sortNodes(node.children);
        }
        return nodes;
    }
    return sortNodes(roots);
}
function highlightMatch(name, term) {
    if (!term) return name;
    const idx = name.toLowerCase().indexOf(term.toLowerCase());
    if (idx === -1) return name;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
        children: [
            name.slice(0, idx),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("mark", {
                className: mark,
                children: name.slice(idx, idx + term.length)
            }),
            name.slice(idx + term.length)
        ]
    });
}
function NodeRenderer({ node, style, gitStatusMap, lineStatsMap, dirStatusMap, loadingPaths, errorPaths, selectedPath, searchTerm, containerWidth }) {
    const data = node.data;
    // Per-node char budget accounts for indent depth (16px per level + 8px base padding).
    const maxChars = Math.floor((containerWidth - 48 - (node.level * 16 + 8)) / 7.5);
    const isSelected = selectedPath === data.id;
    const isLoading = loadingPaths.has(data.id);
    const loadError = errorPaths.get(data.id);
    // Determine git status badge.
    const statusLetter = data.isDir ? dirStatusMap.get(data.id) : gitStatusMap.get(data.id) || data.gitStatus;
    const statusColor = statusLetter ? GIT_STATUS_COLORS[statusLetter] : undefined;
    const lineStats = data.isDir ? undefined : lineStatsMap.get(data.id);
    const icon = data.isSymlink ? "⇢" : data.isDir ? node.isOpen ? "▾" : "▸" : (0,fileIcons/* getFileIcon */.I)(data.name);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        style: style,
        title: data.id,
        className: `${FileTree_css_node} ${isSelected ? selected : ""} ${node.isFocused ? keyboardFocused : ""} ${data.isIgnored ? ignored : ""}`,
        role: "treeitem",
        "aria-level": node.level + 1,
        "aria-selected": isSelected,
        "aria-expanded": data.isDir ? node.isOpen : undefined,
        onClick: ()=>{
            // Directories toggle open/close (fires onToggle → handleToggle → loadDirectory).
            // Files/symlinks activate (fires onActivate → onFileSelect).
            if (data.isDir) node.toggle();
            else node.activate();
        },
        children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: nodeInner,
            style: {
                paddingLeft: `${node.level * 16 + 8}px`
            },
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                    className: FileTree_css_icon,
                    children: icon
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                    className: FileTree_css_name,
                    children: searchTerm ? highlightMatch(data.name, searchTerm) : (0,truncateMiddle/* truncateMiddle */.k)(data.name, maxChars)
                }),
                data.isSymlink && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                    className: symlinkBadge,
                    title: `→ ${data.symlinkTarget}`,
                    children: "symlink"
                }),
                isLoading && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                    className: spinner
                }),
                loadError && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                    className: inlineError,
                    title: loadError,
                    children: "⚠"
                }),
                statusLetter && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                    className: statusBadge,
                    style: {
                        color: statusColor
                    },
                    title: `Git status: ${statusLetter}`,
                    children: statusLetter
                }),
                lineStats && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                    className: FileTree_css_lineStats,
                    title: `+${lineStats.add} -${lineStats.del}`,
                    children: [
                        lineStats.add > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                            className: lineStatsAdd,
                            children: [
                                "+",
                                lineStats.add
                            ]
                        }),
                        lineStats.del > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                            className: lineStatsDel,
                            children: [
                                "-",
                                lineStats.del
                            ]
                        })
                    ]
                })
            ]
        })
    });
}
// ---- Row renderer ----
// Real roving tabindex: react-arborist's DefaultRow hardcodes tabIndex=-1
// forever, which breaks useFocusTrap's [tabindex]:not([tabindex="-1"]) query.
// Exported for unit testing.
function TreeRow({ node, innerRef, attrs, children }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        ...attrs,
        tabIndex: node.isFocused ? 0 : -1,
        ref: innerRef,
        onFocus: (e)=>e.stopPropagation(),
        onClick: node.handleClick,
        children: children
    });
}
// ---- Main component ----
// Stable empty map/set to avoid creating new references on every render when
// the corresponding prop is not provided, which would break useMemo dependency checks.
const EMPTY_GIT_STATUS_MAP = new Map();
const EMPTY_LINE_STATS_MAP = new Map();
const FileTree = /*#__PURE__*/ (0,react.forwardRef)(function FileTree({ sessionId, baseUrl, onFileSelect, gitStatusMap = EMPTY_GIT_STATUS_MAP, lineStatsMap = EMPTY_LINE_STATS_MAP, sortBy = "name", filterChangedOnly = false, selectedPath, includeIgnored = false, searchTerm = "", onSearchResults }, ref) {
    // Map of directory path → loaded TreeNode children.
    const [dirContents, setDirContents] = (0,react.useState)(new Map());
    // Tracks which paths are currently loading.
    // loadingPathsRef is the source of truth; loadingPaths state is kept in sync for renders.
    const loadingPathsRef = (0,react.useRef)(new Set());
    const [loadingPaths, setLoadingPaths] = (0,react.useState)(new Set());
    // Tracks which paths have load errors.
    const [errorPaths, setErrorPaths] = (0,react.useState)(new Map());
    // Root loading/error state.
    const [rootLoading, setRootLoading] = (0,react.useState)(true);
    const [rootError, setRootError] = (0,react.useState)(null);
    // Search mode state. null = browse mode, array = search mode.
    const [searchResults, setSearchResults] = (0,react.useState)(null);
    const [searchLoading, setSearchLoading] = (0,react.useState)(false);
    const [searchTruncated, setSearchTruncated] = (0,react.useState)(false);
    // Request ID ref prevents stale search responses from overwriting newer results.
    const searchRequestIdRef = (0,react.useRef)(0);
    // Timer ref for debouncing search input.
    const searchTimerRef = (0,react.useRef)(null);
    // Track whether we were in search mode to trigger closeAll on exit.
    const wasInSearchModeRef = (0,react.useRef)(false);
    // Snapshot of open node IDs taken just before entering search mode.
    const savedOpenStateRef = (0,react.useRef)({});
    const treeRef = (0,react.useRef)(undefined);
    // Tracks the timestamp of the last 'g' keypress for the gg chord.
    const lastGRef = (0,react.useRef)(0);
    // ResizeObserver: track container dimensions for react-window (requires numeric width/height).
    const containerRef = (0,react.useRef)(null);
    const [dims, setDims] = (0,react.useState)({
        w: 300,
        h: 600
    });
    (0,react.useEffect)(()=>{
        const el = containerRef.current;
        if (!el) return;
        const ro = new ResizeObserver(([entry])=>{
            requestAnimationFrame(()=>{
                const { width, height } = entry.contentRect;
                if (width > 0 && height > 0) {
                    setDims({
                        w: Math.floor(width),
                        h: Math.floor(height)
                    });
                }
            });
        });
        ro.observe(el);
        return ()=>ro.disconnect();
    }, []);
    // Load a directory's children.
    const loadDirectory = (0,react.useCallback)(async (dirPath)=>{
        if (loadingPathsRef.current.has(dirPath)) return;
        loadingPathsRef.current.add(dirPath);
        setLoadingPaths(new Set(loadingPathsRef.current));
        setErrorPaths((prev)=>{
            const next = new Map(prev);
            next.delete(dirPath);
            return next;
        });
        try {
            const response = await (0,useFileService/* fetchDirectoryFiles */.Tl)(sessionId, dirPath === "." ? "." : dirPath, includeIgnored, baseUrl);
            const nodes = (response.files || []).map(fileNodeToTreeNode);
            setDirContents((prev)=>{
                const next = new Map(prev);
                next.set(dirPath, nodes);
                return next;
            });
        } catch (err) {
            const msg = err instanceof Error ? err.message : "Failed to load directory";
            setErrorPaths((prev)=>new Map(prev).set(dirPath, msg));
        } finally{
            loadingPathsRef.current.delete(dirPath);
            setLoadingPaths(new Set(loadingPathsRef.current));
        }
    }, [
        sessionId,
        baseUrl,
        includeIgnored
    ]);
    // Stable ref to latest loadDirectory — allows revealPathImpl to always call the
    // latest version without closing over a stale function instance.
    const loadDirectoryRef = (0,react.useRef)(loadDirectory);
    (0,react.useEffect)(()=>{
        loadDirectoryRef.current = loadDirectory;
    }, [
        loadDirectory
    ]);
    // Load root on mount and whenever session, base URL, or ignored-files setting changes.
    (0,react.useEffect)(()=>{
        setDirContents(new Map());
        setRootLoading(true);
        setRootError(null);
        (0,useFileService/* fetchDirectoryFiles */.Tl)(sessionId, ".", includeIgnored, baseUrl).then((response)=>{
            const nodes = (response.files || []).map(fileNodeToTreeNode);
            setDirContents(new Map([
                [
                    ".",
                    nodes
                ]
            ]));
        }).catch((err)=>{
            setRootError(err instanceof Error ? err.message : "Failed to load files");
        }).finally(()=>{
            setRootLoading(false);
        });
    }, [
        sessionId,
        baseUrl,
        includeIgnored
    ]);
    // Debounced backend search: fires when searchTerm changes.
    (0,react.useEffect)(()=>{
        if (searchTimerRef.current) {
            clearTimeout(searchTimerRef.current);
        }
        if (!searchTerm || searchTerm.length < 2) {
            // Exit search mode.
            setSearchResults(null);
            setSearchLoading(false);
            setSearchTruncated(false);
            onSearchResults?.(null, false);
            return;
        }
        setSearchLoading(true);
        searchTimerRef.current = setTimeout(async ()=>{
            const requestId = ++searchRequestIdRef.current;
            try {
                const response = await (0,useFileService/* searchFiles */._K)(sessionId, searchTerm, includeIgnored, baseUrl);
                if (requestId !== searchRequestIdRef.current) return; // stale response
                const tree = buildSearchTree(response.files || []);
                setSearchResults(tree);
                setSearchTruncated(response.truncated);
                setSearchLoading(false);
                onSearchResults?.(response.totalMatches, response.truncated);
            } catch  {
                if (requestId !== searchRequestIdRef.current) return;
                setSearchResults([]);
                setSearchLoading(false);
                onSearchResults?.(0, false);
            }
        }, 300);
        return ()=>{
            if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
        };
    }, [
        searchTerm,
        sessionId,
        includeIgnored,
        baseUrl
    ]); // eslint-disable-line react-hooks/exhaustive-deps
    // Open all tree nodes when entering search mode; restore prior state when leaving.
    (0,react.useEffect)(()=>{
        if (searchResults !== null) {
            if (!wasInSearchModeRef.current) {
                // Snapshot expanded state before entering search for restoration on exit.
                savedOpenStateRef.current = {
                    ...treeRef.current?.openState ?? {}
                };
                wasInSearchModeRef.current = true;
            }
            // Delay to allow react-arborist to render the new data before calling openAll.
            const timer = setTimeout(()=>{
                treeRef.current?.openAll();
            }, 0);
            return ()=>clearTimeout(timer);
        } else if (wasInSearchModeRef.current) {
            wasInSearchModeRef.current = false;
            // Restore the browse-mode open state instead of collapsing everything.
            const saved = savedOpenStateRef.current;
            treeRef.current?.closeAll();
            for (const [id, isOpen] of Object.entries(saved)){
                if (isOpen) treeRef.current?.open(id);
            }
            savedOpenStateRef.current = {};
        }
    }, [
        searchResults
    ]);
    // Memoized tree computations — only recompute when their inputs actually change.
    const treeData = (0,react.useMemo)(()=>buildTreeData(dirContents.get(".") ?? [], dirContents), [
        dirContents
    ]);
    const browsedData = (0,react.useMemo)(()=>searchResults ?? treeData, [
        searchResults,
        treeData
    ]);
    const changedAncestorPrefixes = (0,react.useMemo)(()=>buildChangedAncestorPrefixes(gitStatusMap), [
        gitStatusMap
    ]);
    const filteredData = (0,react.useMemo)(()=>filterChangedOnly ? filterChangedTreeData(browsedData, gitStatusMap, changedAncestorPrefixes) : browsedData, [
        browsedData,
        filterChangedOnly,
        gitStatusMap,
        changedAncestorPrefixes
    ]);
    const displayedData = (0,react.useMemo)(()=>sortTreeData(filteredData, sortBy), [
        filteredData,
        sortBy
    ]);
    const dirStatusMap = (0,react.useMemo)(()=>{
        const m = new Map();
        computeDirStatuses(displayedData, gitStatusMap, m);
        return m;
    }, [
        displayedData,
        gitStatusMap
    ]);
    // Set of all known directory ids across all loaded children.
    // Allows handleToggle to skip non-directory ids without re-traversing the tree.
    const knownDirIds = (0,react.useMemo)(()=>{
        const s = new Set();
        for (const nodes of dirContents.values()){
            for (const node of nodes){
                if (node.isDir) s.add(node.id);
            }
        }
        return s;
    }, [
        dirContents
    ]);
    // Row height: 44px on narrow (mobile) viewports for touch target compliance, 28px otherwise.
    const rowHeight = dims.w < 640 ? 44 : 28;
    // AbortController for in-flight revealPath operations — ensures only one runs at a time.
    const revealAbortRef = (0,react.useRef)(null);
    // Expand ancestor directories and scroll to targetPath.
    const revealPathImpl = (0,react.useCallback)(async (targetPath)=>{
        revealAbortRef.current?.abort();
        const controller = new AbortController();
        revealAbortRef.current = controller;
        const signal = controller.signal;
        // Build ancestor paths: e.g. 'a/b/c.tsx' → ['a', 'a/b']
        const segments = targetPath.split("/");
        const ancestors = [];
        for(let i = 1; i < segments.length; i++){
            ancestors.push(segments.slice(0, i).join("/"));
        }
        for (const ancestorPath of ancestors){
            if (signal.aborted) return;
            // Load if not already loaded.
            if (!dirContents.has(ancestorPath)) {
                try {
                    await loadDirectoryRef.current(ancestorPath);
                    await new Promise((r)=>setTimeout(r, 0));
                } catch  {
                    // Directory load failed; stop reveal at this ancestor
                    return;
                }
            }
            // Open the directory in the tree.
            treeRef.current?.open(ancestorPath);
        }
        if (signal.aborted) return;
        // Scroll the target node into view.
        requestAnimationFrame(()=>{
            treeRef.current?.scrollTo(targetPath);
        });
    }, [
        dirContents
    ]);
    // Expose imperative handle to parent refs.
    (0,react.useImperativeHandle)(ref, ()=>({
            collapseAll: ()=>{
                treeRef.current?.closeAll();
            },
            revealPath: (path)=>{
                void revealPathImpl(path);
            }
        }), [
        revealPathImpl
    ]);
    const handleActivate = (0,react.useCallback)((node)=>{
        const data = node.data;
        if (!data.isDir && !data.isSymlink) {
            onFileSelect(data.id);
        }
    }, [
        onFileSelect
    ]);
    const handleToggle = (0,react.useCallback)((id)=>{
        // load-bearing guard: prevents openAll() fan-out in search mode.
        if (searchResults !== null) return;
        // Only attempt to load known directory ids; ignore files and unknown ids.
        if (!knownDirIds.has(id)) return;
        // Load if not yet fetched, or retry if a previous load errored.
        if (!dirContents.has(id) || errorPaths.has(id)) {
            loadDirectory(id);
        }
    }, [
        dirContents,
        errorPaths,
        knownDirIds,
        loadDirectory,
        searchResults
    ]);
    const handleTreeKeyDown = (0,react.useCallback)((e)=>{
        const tree = treeRef.current;
        if (!tree) return;
        // Don't intercept modified shortcuts (Ctrl+G, Cmd+K, Alt+j, etc.)
        if (e.metaKey || e.ctrlKey || e.altKey) return;
        const focusedNode = tree.focusedNode;
        const visible = tree.visibleNodes;
        if (!visible || visible.length === 0) return;
        switch(e.key){
            case "Tab":
                {
                    // react-arborist's own DefaultContainer already moved DOM focus via
                    // its document-wide walk (not safely overridable — depends on
                    // unexported internals). Redo the move correctly using the row it
                    // moved focus from: step within the tree like j/k, or at a boundary
                    // hand off to our own container so useFocusTrap resolves the wrap.
                    if (!focusedNode) break;
                    e.preventDefault();
                    const idx = visible.findIndex((n)=>n.id === focusedNode.id);
                    if (e.shiftKey) {
                        const prev = idx > 0 ? visible[idx - 1] : undefined;
                        if (prev) tree.focus(prev.id);
                        else containerRef.current?.focus();
                    } else {
                        const next = idx >= 0 && idx < visible.length - 1 ? visible[idx + 1] : undefined;
                        if (next) tree.focus(next.id);
                        else containerRef.current?.focus();
                    }
                    break;
                }
            case "j":
                {
                    e.preventDefault();
                    const idx = focusedNode ? visible.findIndex((n)=>n.id === focusedNode.id) : -1;
                    const next = visible[idx + 1];
                    if (next) tree.focus(next.id);
                    break;
                }
            case "k":
                {
                    e.preventDefault();
                    const idx = focusedNode ? visible.findIndex((n)=>n.id === focusedNode.id) : -1;
                    const prev = visible[Math.max(0, idx - 1)];
                    if (prev) tree.focus(prev.id);
                    break;
                }
            case "l":
                {
                    e.preventDefault();
                    if (!focusedNode) break;
                    if (focusedNode.data.isDir) {
                        tree.open(focusedNode.id);
                    } else {
                        focusedNode.activate();
                    }
                    break;
                }
            case "h":
                {
                    e.preventDefault();
                    if (!focusedNode) break;
                    if (focusedNode.data.isDir && focusedNode.isOpen) {
                        tree.close(focusedNode.id);
                    } else if (focusedNode.parent && !focusedNode.parent.isRoot) {
                        tree.focus(focusedNode.parent.id);
                    }
                    break;
                }
            case "g":
                {
                    e.preventDefault();
                    const now = Date.now();
                    if (now - lastGRef.current < 400) {
                        const first = visible[0];
                        if (first) tree.focus(first.id);
                        lastGRef.current = 0;
                    } else {
                        lastGRef.current = now;
                    }
                    break;
                }
            case "G":
                {
                    e.preventDefault();
                    const last = visible[visible.length - 1];
                    if (last) tree.focus(last.id);
                    break;
                }
            case "Enter":
                {
                    if (!focusedNode) break;
                    e.preventDefault();
                    if (focusedNode.data.isDir) {
                        focusedNode.toggle();
                    } else {
                        focusedNode.activate();
                    }
                    break;
                }
            default:
                break;
        }
    }, [] // treeRef and lastGRef are refs — stable, no deps needed
    );
    const retryRoot = ()=>{
        setRootLoading(true);
        setRootError(null);
        (0,useFileService/* fetchDirectoryFiles */.Tl)(sessionId, ".", includeIgnored, baseUrl).then((response)=>setDirContents(new Map([
                [
                    ".",
                    (response.files || []).map(fileNodeToTreeNode)
                ]
            ]))).catch((err)=>setRootError(err instanceof Error ? err.message : "Failed to load files")).finally(()=>setRootLoading(false));
    };
    const showTreePending = (0,PendingState/* usePendingDelay */.oQ)(rootLoading);
    if (rootLoading) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
            className: container,
            children: showTreePending && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(PendingState/* WaitState */.Dt, {
                kind: "tree"
            })
        });
    }
    if (rootError) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
            className: container,
            children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(PendingState/* WaitState */.Dt, {
                kind: "tree",
                failure: {
                    reason: "rpc_error",
                    actions: [
                        {
                            label: "Retry",
                            onClick: retryRoot
                        }
                    ]
                }
            })
        });
    }
    // Search empty state.
    if (searchResults !== null && searchResults.length === 0) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
            className: container,
            children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: searchEmpty,
                children: [
                    "No files match “",
                    searchTerm,
                    "”"
                ]
            })
        });
    }
    if (treeData.length === 0 && searchResults === null) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
            className: container,
            children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: empty,
                children: "This directory is empty."
            })
        });
    }
    // Filter-to-changed empty state — distinct from "directory is empty" above.
    if (filterChangedOnly && displayedData.length === 0) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
            className: container,
            children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: empty,
                children: "No changed files."
            })
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: container,
        ref: containerRef,
        tabIndex: 0,
        onKeyDown: handleTreeKeyDown,
        role: "tree",
        title: "Navigate with j/k/h/l  •  Enter to open  •  gg/G for top/bottom",
        "aria-label": "File tree. Use j/k to navigate, l/Enter to open, h to go up.",
        children: [
            searchTruncated && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: FileTree_css_searchTruncated,
                children: "Showing first 500 results — refine your search for more specific matches."
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                style: {
                    position: "relative",
                    flex: 1
                },
                children: [
                    searchLoading && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: searchOverlay,
                        children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            className: spinner
                        })
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)(tree/* Tree */.P, {
                        ref: treeRef,
                        data: displayedData,
                        idAccessor: (node)=>node.id,
                        childrenAccessor: (node)=>{
                            if (!node.isDir) return null;
                            // Returning [] (not null) for all dirs keeps isLeaf=false so node.toggle() works.
                            // Returning null would make the node a leaf and prevent any toggle from firing.
                            return node.children ?? [];
                        },
                        disableDrag: true,
                        disableDrop: true,
                        renderRow: TreeRow,
                        onActivate: handleActivate,
                        onToggle: handleToggle,
                        rowHeight: rowHeight,
                        openByDefault: false,
                        width: dims.w,
                        height: dims.h,
                        searchTerm: searchResults === null ? searchTerm || undefined : undefined,
                        searchMatch: (node, term)=>{
                            const t = term.toLowerCase();
                            return node.data.name.toLowerCase().includes(t) || node.data.id.toLowerCase().includes(t);
                        },
                        children: ({ node, style, dragHandle })=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)(NodeRenderer, {
                                node: node,
                                style: style,
                                dragHandle: dragHandle,
                                gitStatusMap: gitStatusMap,
                                lineStatsMap: lineStatsMap,
                                dirStatusMap: dirStatusMap,
                                loadingPaths: loadingPaths,
                                errorPaths: errorPaths,
                                selectedPath: selectedPath,
                                includeIgnored: includeIgnored,
                                searchTerm: searchTerm,
                                containerWidth: dims.w
                            })
                    })
                ]
            })
        ]
    });
});


/***/ }),

/***/ 96952:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Es: () => (/* binding */ stepFontPx),
/* harmony export */   L9: () => (/* binding */ useFontPx)
/* harmony export */ });
/* unused harmony exports DESKTOP_DEFAULT_PX, COARSE_MIN_PX, DESKTOP_MIN_PX, MAX_PX, defaultFontPx, clampFontPx, readFontPx */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* __next_internal_client_entry_do_not_use__ DESKTOP_DEFAULT_PX,COARSE_MIN_PX,DESKTOP_MIN_PX,MAX_PX,defaultFontPx,clampFontPx,readFontPx,stepFontPx,useFontPx auto */ 
const KEY = "ssq.files.fontPx";
const CHANGE = "ssq-files-font-change";
const DESKTOP_DEFAULT_PX = 13;
/** Soft keyboards zoom the page when a focused field renders below 16px. */ const COARSE_MIN_PX = 16;
const DESKTOP_MIN_PX = 8;
const MAX_PX = 28;
const defaultFontPx = (coarse)=>coarse ? COARSE_MIN_PX : DESKTOP_DEFAULT_PX;
const clampFontPx = (px, coarse)=>Math.min(MAX_PX, Math.max(coarse ? COARSE_MIN_PX : DESKTOP_MIN_PX, Math.round(px)));
/** Set only while storage rejects writes, so the step still applies for this session. */ let unsavedPx = null;
/** The stepped size for this device, or the default; never below the touch floor on a coarse pointer. */ function readFontPx(coarse) {
    if (unsavedPx !== null) return clampFontPx(unsavedPx, coarse);
    try {
        const stored = Number(window.localStorage.getItem(KEY));
        if (Number.isFinite(stored) && stored > 0) return clampFontPx(stored, coarse);
    } catch  {
    // storage unavailable: default
    }
    return defaultFontPx(coarse);
}
/** One 1px step (A+ / A-), persisted per device. Returns the new size. */ function stepFontPx(delta, coarse) {
    const next = clampFontPx(readFontPx(coarse) + delta, coarse);
    try {
        window.localStorage.setItem(KEY, String(next));
        unsavedPx = null;
    } catch  {
        unsavedPx = next;
    }
    window.dispatchEvent(new Event(CHANGE));
    return next;
}
const subscribe = (cb)=>{
    window.addEventListener(CHANGE, cb);
    window.addEventListener("storage", cb);
    return ()=>{
        window.removeEventListener(CHANGE, cb);
        window.removeEventListener("storage", cb);
    };
};
function useFontPx(coarse) {
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useSyncExternalStore)(subscribe, ()=>readFontPx(coarse), ()=>defaultFontPx(coarse));
}


/***/ }),

/***/ 98222:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   E: () => (/* binding */ useWatchBacklogItems)
/* harmony export */ });
/* harmony import */ var _deployId__WEBPACK_IMPORTED_MODULE_8__ = __webpack_require__(39351);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(4435);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_7__ = __webpack_require__(73396);
/* harmony import */ var _gen_session_v1_backlog_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(34548);
/* harmony import */ var _lib_store__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(92360);
/* harmony import */ var _lib_store_backlogItemsSlice__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(43457);
/* harmony import */ var _lib_hooks_useBacklogService__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(78120);
/* harmony import */ var _lib_hooks_useWatchStream__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(51275);
/* __next_internal_client_entry_do_not_use__ useWatchBacklogItems auto */ 








const MAX_RETRIES = 5;
const FALLBACK_POLL_INTERVAL_MS = 30000;
// AC #19 (project_plans/backlog-event-driven-updates/design/ux.md,
// ConnectionIndicator's "Rapid connect/disconnect flapping" edge case): only
// the reconnecting -> live transition is debounced by a "few hundred ms"
// hold, never connected -> reconnecting (that must announce trouble
// immediately). A flaky connection that reconnects and drops again within
// this window never visibly flickers back to "Live".
const LIVE_TRANSITION_DEBOUNCE_MS = 300;
/**
 * Defense-in-depth patch for the verdict_recorded event (see the switch case
 * below): finds the most recent review-role ItemSession in `item.itemSessions`
 * — the same session mapBacklogItem's own "most recent review session" logic
 * (useBacklogService.ts) reads gateVerdict/gateVerdictSummary from — and, if
 * its embedded reviewVerdict doesn't already match, overwrites it with the
 * event's inline `verdict` field. Returns `item` unchanged (same reference)
 * when there is nothing to patch, so callers that don't need this stay cheap.
 *
 * Deliberately does NOT fabricate a review session when none exists in the
 * embedded item — that gap is deeper than a missing verdict field, and
 * inventing session metadata (id, sessionUuid, startedAt, ...) this hook has
 * no visibility into would be worse than leaving it to the primary
 * (server-side eager-load) fix path.
 */ function applyInlineVerdict(item, verdict) {
    if (!verdict) return item;
    const sessions = item.itemSessions;
    let lastReviewIdx = -1;
    for(let i = 0; i < sessions.length; i++){
        if (sessions[i].sessionRole === "review") lastReviewIdx = i;
    }
    if (lastReviewIdx === -1) return item;
    const existing = sessions[lastReviewIdx];
    if (existing.reviewVerdict?.overallOutcome === verdict.overallOutcome && existing.reviewVerdict?.summary === verdict.summary) {
        return item;
    }
    const patchedSessions = sessions.slice();
    patchedSessions[lastReviewIdx] = {
        ...existing,
        reviewVerdict: verdict
    };
    return {
        ...item,
        itemSessions: patchedSessions
    };
}
/**
 * Subscribes to real-time backlog item changes via the WatchBacklogItems
 * streaming RPC, dispatching every received event into backlogItemsSlice,
 * and keeps the store fresh across disconnects (exponential-backoff
 * reconnect, REST fallback polling, after_seq replay, and an idle-staleness
 * backstop for periods with zero live events).
 */ function useWatchBacklogItems(filters = {}) {
    const { statusFilter, categoryFilter } = filters;
    // Stable string keys so effects don't re-run just because the caller
    // passed a fresh array/object literal this render (a likely pitfall for a
    // hook that Phase 5 will call inline in component bodies).
    const statusFilterKey = (statusFilter ?? []).join(",");
    const categoryFilterKey = (categoryFilter ?? []).join(",");
    const dispatch = (0,_lib_store__WEBPACK_IMPORTED_MODULE_2__/* .useAppDispatch */ .jL)();
    const items = (0,_lib_store__WEBPACK_IMPORTED_MODULE_2__/* .useAppSelector */ .GV)(_lib_store_backlogItemsSlice__WEBPACK_IMPORTED_MODULE_3__/* .selectAllBacklogItems */ .Bi);
    const liveVersions = (0,_lib_store__WEBPACK_IMPORTED_MODULE_2__/* .useAppSelector */ .GV)(_lib_store_backlogItemsSlice__WEBPACK_IMPORTED_MODULE_3__/* .selectBacklogItemsLiveVersionMap */ .Ky);
    const [connectionState, setConnectionState] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)("connecting");
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    // Gap-detection bookkeeping (Story 4.2.2): distinct from useWatchStream's
    // own internal afterSeq tracking -- this one drives triggerResync's
    // forward/backward-jump full-refetch logic, a backlog-specific business
    // rule useWatchStream has no reason to know about.
    const lastSeqRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(0n);
    const lastDeployIdRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)("");
    const resyncInFlightRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(false);
    // Set when useWatchStream reports "stale" (a watchdog- or visibility-
    // triggered reconnect); cleared (and a full refetch fired) on that
    // reconnect's "live" transition. Ties the "reconnect success path issues a
    // refetch" requirement (Story 4.2.3) specifically to self-healing
    // reconnects, not ordinary in-loop backoff retries.
    const staleReconnectPendingRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(false);
    // Mirrors whether useWatchStream is currently reporting "live", so
    // scheduleLiveTransition's debounced commit (AC #19) can bail out if the
    // connection has already dropped again by the time its timer fires.
    const isLiveRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(false);
    // AC #19: pending reconnecting -> live debounce (see
    // LIVE_TRANSITION_DEBOUNCE_MS above). Re-armed on every successful
    // (re)connect/resync; the scheduled flip to "live" only actually commits if
    // the stream is still connected once the timer fires, so a flap that drops
    // again mid-hold never shows "Live" at all.
    const liveTransitionTimerRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    // Initialize ConnectRPC client via the shared watch-transport singleton
    // (server.go now registers BacklogService.WatchBacklogItems with
    // StreamingWSBridge, same as useSessionService/useReviewQueue) — avoids
    // holding one more long-lived HTTP/1.1 connection against the browser's
    // 6-connections-per-origin budget.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .createClient */ .UU)(_gen_session_v1_backlog_pb__WEBPACK_IMPORTED_MODULE_1__/* .BacklogService */ .jSM, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_7__/* .getWatchTransport */ .VZ)());
    }, []);
    // Full REST refetch — used for the initial load, gap-detected resyncs, the
    // fallback poll, and every successful (re)connection after the first.
    const refresh = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (!clientRef.current) return;
        try {
            const resp = await clientRef.current.listBacklogItems({
                // ListBacklogItemsRequest has no category field (see
                // backlogItemMatchesFilters's doc comment server-side) — only status
                // can be enforced on this REST snapshot; category filtering only
                // applies to the live stream below.
                status: statusFilter ?? [],
                priority: [],
                includeTerminal: false,
                includeArchived: false,
                sortBy: ""
            });
            for (const item of resp.items ?? []){
                dispatch((0,_lib_store_backlogItemsSlice__WEBPACK_IMPORTED_MODULE_3__/* .upsertItem */ .vH)(item));
            }
        } catch (err) {
            console.error("[useWatchBacklogItems] listBacklogItems failed:", err);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        dispatch,
        statusFilterKey
    ]);
    // On mount, immediately issue the REST snapshot fetch alongside (not
    // gated behind) opening the stream below — Task 4.2.1a/pitfalls.md #1.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        statusFilterKey
    ]);
    // AC #19: debounce the reconnecting -> live transition only (never
    // connected -> reconnecting, which must show immediately elsewhere in this
    // hook). Cancels any previously scheduled flip and re-arms the hold, so
    // repeated flapping keeps deferring "Live" until the connection has been
    // stable for the full window rather than flickering it on and off.
    const scheduleLiveTransition = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        if (liveTransitionTimerRef.current) clearTimeout(liveTransitionTimerRef.current);
        liveTransitionTimerRef.current = setTimeout(()=>{
            liveTransitionTimerRef.current = null;
            if (isLiveRef.current) setConnectionState("live");
        }, LIVE_TRANSITION_DEBOUNCE_MS);
    }, []);
    // Gap-detected resync: briefly reflect a "reconnecting" state while the
    // refetch is in flight, then return to "live" if the stream is still up.
    const triggerResync = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (resyncInFlightRef.current) return;
        resyncInFlightRef.current = true;
        setConnectionState((prev)=>prev === "live" ? "reconnecting" : prev);
        try {
            await refresh();
        } finally{
            resyncInFlightRef.current = false;
            if (isLiveRef.current) scheduleLiveTransition();
        }
    }, [
        refresh,
        scheduleLiveTransition
    ]);
    // Apply after_seq bookkeeping (Story 4.2.2) then dispatch to the store.
    const handleEvent = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((event)=>{
        // seq === 0 marks a synthetic per-item snapshot event sent on a fresh
        // (non-replay) connection — it carries no real bus sequence number, so
        // it must not participate in gap detection (see BacklogItemEvent.seq's
        // proto doc comment).
        const seq = event.seq;
        if ((0,_deployId__WEBPACK_IMPORTED_MODULE_8__/* .noteDeployId */ .j)(lastDeployIdRef, event.deployId, "backlog-items")) {
            // New server process: seq restarted, so the old cursor is meaningless. Resync, then re-seed below.
            lastSeqRef.current = 0n;
            void triggerResync();
        }
        if (seq > 0n) {
            const prev = lastSeqRef.current;
            if (prev > 0n) {
                if (seq < prev) {
                    // Backwards jump: server restarted and its seq counter reset.
                    // Mirrors useSessionService.ts:730-742 exactly.
                    lastSeqRef.current = 0n;
                    void triggerResync();
                } else if (seq !== prev + 1n) {
                    // Forward gap: the bus's bounded, non-blocking fan-out dropped
                    // an event under backpressure (pitfalls.md #4 — new logic, not
                    // present in useSessionService.ts today).
                    lastSeqRef.current = seq;
                    void triggerResync();
                } else {
                    lastSeqRef.current = seq;
                }
            } else {
                // No established baseline yet (very first real-seq event this
                // client has ever seen) — seed it without treating an arbitrary
                // starting seq as a gap.
                lastSeqRef.current = seq;
            }
        }
        switch(event.event.case){
            case "verdictRecorded":
                {
                    // Defense-in-depth (Phase 5 spec-compliance sweep,
                    // backlog-event-driven-updates): BacklogItemVerdictRecordedEvent
                    // carries the just-saved verdict inline (`verdict`), populated
                    // directly from the ReviewVerdictData the backend just wrote —
                    // independent of whether the embedded `item.itemSessions` snapshot
                    // was eager-loaded correctly (the root-cause fix for that lives
                    // server-side, session/ent_repository_backlog.go's
                    // attachItemSessionsForPublish). Patch the inline verdict onto the
                    // most recent review-role session in the embedded item before
                    // dispatching, so gateVerdict/gateVerdictSummary (mapBacklogItem,
                    // useBacklogService.ts) get a second, independent path to
                    // correctness even if that primary path has a gap.
                    const { item, verdict, isSnapshot } = event.event.value;
                    if (item) dispatch((0,_lib_store_backlogItemsSlice__WEBPACK_IMPORTED_MODULE_3__/* .upsertItem */ .vH)(applyInlineVerdict(item, verdict), isSnapshot));
                    break;
                }
            case "statusChanged":
            case "sessionAttached":
            case "itemUpdated":
                {
                    const item = event.event.value.item;
                    // Epic 6.1: thread this event's own is_snapshot flag through so
                    // the store only bumps liveVersion (flash-eligible) for a genuine
                    // live event — never for the initial snapshot or a forced-
                    // is_snapshot replay-branch copy (pre-mortem #4).
                    if (item) dispatch((0,_lib_store_backlogItemsSlice__WEBPACK_IMPORTED_MODULE_3__/* .upsertItem */ .vH)(item, event.event.value.isSnapshot));
                    break;
                }
            case "itemArchived":
                break;
            case "itemRemoved":
                dispatch((0,_lib_store_backlogItemsSlice__WEBPACK_IMPORTED_MODULE_3__/* .removeItem */ .Ai)(event.event.value.itemId));
                break;
            case "activityNoteAdded":
                {
                    // ADR-002: a dedicated single-entry event — never a full item
                    // snapshot — so it dispatches the targeted appendActivityNote
                    // reducer, never upsertItem.
                    const { itemId, note } = event.event.value;
                    if (note) dispatch((0,_lib_store_backlogItemsSlice__WEBPACK_IMPORTED_MODULE_3__/* .appendActivityNote */ .Se)({
                        itemId,
                        note
                    }));
                    break;
                }
            case "snapshotComplete":
                break;
            default:
                break;
        }
    }, [
        dispatch,
        triggerResync
    ]);
    // Connection mechanics (backoff reconnect, idle-staleness watchdog) live in
    // the shared useWatchStream.ts; this only supplies the request shape,
    // afterSeq bookkeeping (shared with the gap-detection logic in handleEvent
    // above via lastSeqRef), and the AC #19 / Story 4.2.3 presentation layer
    // on top of its connection-state callback.
    const subscribeBacklogItems = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((afterSeq, signal)=>clientRef.current.watchBacklogItems({
            statusFilter: statusFilter ?? [],
            categoryFilter: categoryFilter ?? [],
            afterSeq
        }, {
            signal
        }), [
        statusFilter,
        categoryFilter
    ]);
    const onConnectionStateChange = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((next)=>{
        isLiveRef.current = next === "live";
        switch(next){
            case "live":
                scheduleLiveTransition();
                // Story 4.2.3: a watchdog- or visibility-triggered reconnect's
                // success path issues a full refetch, even if zero
                // BacklogItemEvents were ever received during the whole idle
                // period beforehand.
                if (staleReconnectPendingRef.current) {
                    staleReconnectPendingRef.current = false;
                    void refresh();
                }
                break;
            case "stale":
                staleReconnectPendingRef.current = true;
                setConnectionState("stale");
                break;
            case "reconnecting":
                setConnectionState("reconnecting");
                break;
            case "exhausted":
                setConnectionState("polling");
                break;
            case "connecting":
                break;
        }
    }, [
        refresh,
        scheduleLiveTransition
    ]);
    const { reconnect } = (0,_lib_hooks_useWatchStream__WEBPACK_IMPORTED_MODULE_5__/* .useWatchStream */ .t)({
        subscribe: subscribeBacklogItems,
        onEvent: handleEvent,
        isHeartbeat: (event)=>event.heartbeat,
        getSeq: (event)=>event.seq,
        onConnectionStateChange,
        maxRetries: MAX_RETRIES,
        // Preserves this hook's original two-tier Story 4.2.3 staleness design:
        // the periodic background watchdog tolerates 30s (matching the original
        // BACKSTOP_INTERVAL_MS, and wide enough not to fire mid-backoff during
        // the MAX_RETRIES exhaustion sequence above), while a tab-refocus/online
        // event reacts faster at 15s (STALE_THRESHOLD_MS) since a user actively
        // looking at the tab justifies a snappier check than a background poll.
        staleThresholdMs: 30000,
        visibilityStaleThresholdMs: 15000,
        // Restart the connection (fresh afterSeq=0) when the filters actually
        // change value, not just array identity -- subscribeBacklogItems reads
        // statusFilter/categoryFilter live via optionsRef otherwise, which would
        // silently keep the stale filter set on an existing connection.
        restartKey: `${statusFilterKey}|${categoryFilterKey}`
    });
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        return ()=>{
            if (liveTransitionTimerRef.current) {
                clearTimeout(liveTransitionTimerRef.current);
                liveTransitionTimerRef.current = null;
            }
        };
    }, []);
    // REST fallback polling (Task 4.2.1d): once useWatchStream reports
    // "exhausted" (MAX_RETRIES backoff attempts failed), poll periodically; a
    // successful poll attempts exactly one reconnect before continuing to poll.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const interval = setInterval(()=>{
            void (async ()=>{
                await refresh();
                if (connectionState === "polling") reconnect();
            })();
        }, FALLBACK_POLL_INTERVAL_MS);
        return ()=>clearInterval(interval);
    }, [
        refresh,
        reconnect,
        connectionState
    ]);
    // Map proto -> domain shape at the hook boundary (see file header, note 3)
    // so every consumer gets the same fields useBacklogService.listBacklogItems
    // already produces, rather than each render component reimplementing
    // (and risking drift from) mapBacklogItem's derivation logic.
    //
    // Cached per source proto-item reference (not just per useMemo call) —
    // Epic 6.1's render-count guarantee (pre-mortem #3) needs each *unrelated*
    // item's mapped object to keep the same identity across a store update,
    // so a memoized consumer (BacklogItemCard, wrapped in React.memo) actually
    // skips re-rendering it. selectAllBacklogItems must return a brand-new
    // array whenever ANY item changes (see its own doc comment), but thanks to
    // Immer's structural sharing, an unrelated item's *element* reference
    // inside that array is untouched — plain `.map(mapBacklogItem)` would
    // still call the mapper (and allocate a fresh object) for every element on
    // every call, throwing that stability away. This WeakMap keyed on the
    // proto item reference restores it: only items whose proto reference
    // actually changed (i.e. were upserted) get remapped, and that's exactly
    // where the fresh `liveVersion` value belongs too.
    const mappedItemCacheRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(new WeakMap());
    const mappedItems = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>{
        const cache = mappedItemCacheRef.current;
        return items.map((protoItem)=>{
            const cached = cache.get(protoItem);
            if (cached) return cached;
            const mapped = {
                ...(0,_lib_hooks_useBacklogService__WEBPACK_IMPORTED_MODULE_4__/* .mapBacklogItem */ .B5)(protoItem),
                liveVersion: liveVersions[protoItem.id]
            };
            cache.set(protoItem, mapped);
            return mapped;
        });
    }, [
        items,
        liveVersions
    ]);
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>({
            items: mappedItems,
            connectionState
        }), [
        mappedItems,
        connectionState
    ]);
}


/***/ })

};
;