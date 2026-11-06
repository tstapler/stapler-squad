"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[3889],{

/***/ 8569:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   B6: () => (/* binding */ isInputElement),
/* harmony export */   u5: () => (/* binding */ registry)
/* harmony export */ });
/* unused harmony exports RESERVED_SEQUENCE_KEYS, ShortcutRegistry */
/** Keys the command bar reserves for list navigation; never valid in a sequence. */ const RESERVED_SEQUENCE_KEYS = new Set([
    "j",
    "k",
    "?"
]);
/**
 * ShortcutRegistry — centralized keyboard shortcut manager.
 *
 * - Single document.addEventListener("keydown") for all shortcuts.
 * - Context-sensitive: detects active context via data-context attribute
 *   on the focused element's ancestor chain.
 * - IME composition events are skipped for single-character shortcuts.
 * - Conflict detection: warns (does not throw) on duplicate id registration.
 */ class ShortcutRegistry {
    /**
   * Register a shortcut. Returns a cleanup function that deregisters it.
   */ register(id, shortcut) {
        if (shortcut.sequence) {
            if (shortcut.sequence.length === 0 || shortcut.sequence.some((k)=>k.length !== 1 || RESERVED_SEQUENCE_KEYS.has(k))) {
                console.warn('[ShortcutRegistry] Invalid sequence for "'.concat(id, '": ').concat(JSON.stringify(shortcut.sequence), " — ignoring sequence."));
                shortcut = {
                    ...shortcut,
                    sequence: undefined
                };
            }
        }
        if (this.shortcuts.has(id)) {
            console.warn('[ShortcutRegistry] Duplicate shortcut id: "'.concat(id, '" — overwriting.'));
        }
        this.shortcuts.set(id, shortcut);
        return ()=>{
            this.shortcuts.delete(id);
        };
    }
    /**
   * Get all registered shortcuts grouped by context.
   * Used by the ? overlay to render the shortcut reference.
   */ getAll() {
        const result = {
            global: [],
            "session-list": [],
            approval: [],
            terminal: [],
            cockpit: [],
            omnibar: []
        };
        for (const shortcut of this.shortcuts.values()){
            if (shortcut.hidden) continue;
            result[shortcut.context].push(shortcut);
        }
        return result;
    }
    /**
   * Every visible action, for the command bar's fuzzy palette: includes actions
   * with no sequence (they show their direct chord). Stable registration order.
   */ getPalette() {
        const out = [];
        for (const [id, shortcut] of this.shortcuts){
            if (!shortcut.hidden) out.push({
                id,
                shortcut
            });
        }
        return out;
    }
    /**
   * Group-nested which-key structure built from `sequence` fields. A node is
   * either a leaf (the sequence ends there) or a branch; if one registration's
   * sequence is a prefix of another's, the longer one wins the key and the
   * shorter is dropped (warned) so a key never both runs and descends.
   */ getTree() {
        const root = {
            key: "",
            label: "",
            children: []
        };
        // Longest sequences first so a branch always beats a leaf that is its prefix,
        // independent of React effect registration order.
        const entries = [
            ...this.shortcuts.entries()
        ].filter((param)=>{
            let [, sc] = param;
            return sc.sequence && !sc.hidden;
        }).sort((param, param1)=>{
            let [, a] = param, [, b] = param1;
            return b.sequence.length - a.sequence.length;
        });
        for (const [id, shortcut] of entries){
            const seq = shortcut.sequence;
            let node = root;
            let skipped = false;
            for(let i = 0; i < seq.length; i++){
                const key = seq[i];
                const isLast = i === seq.length - 1;
                let child = node.children.find((c)=>c.key === key);
                if (child === null || child === void 0 ? void 0 : child.shortcut) {
                    skipped = true; // would extend (or duplicate) a leaf
                    break;
                }
                if (!child) {
                    var _shortcut_group;
                    child = {
                        key,
                        label: i === 0 ? (_shortcut_group = shortcut.group) !== null && _shortcut_group !== void 0 ? _shortcut_group : key : key,
                        children: []
                    };
                    node.children.push(child);
                } else if (i === 0 && shortcut.group && child.label === child.key) {
                    child.label = shortcut.group;
                }
                if (isLast) {
                    if (child.children.length > 0) {
                        skipped = true; // a longer sequence already owns this key as a branch
                        break;
                    }
                    child.shortcut = shortcut;
                    child.label = shortcut.label;
                }
                node = child;
            }
            if (skipped) console.warn("[ShortcutRegistry] Sequence ".concat(JSON.stringify(seq), ' of "').concat(id, '" collides with another binding — skipped.'));
        }
        return root.children;
    }
    /**
   * Register a provider whose shortcuts exist only while the command bar is under
   * `prefix` (called by resolveLazy). Keeps per-item actions (e.g. reply options
   * 1..N) off the registry at mount.
   */ registerLazy(id, prefix, provider) {
        const entry = {
            prefix,
            provider,
            cleanups: []
        };
        this.lazyProviders.set(id, entry);
        return ()=>{
            entry.cleanups.forEach((c)=>c());
            this.lazyProviders.delete(id);
        };
    }
    /** Drop every lazily provided shortcut (the bar closed). */ releaseLazy() {
        for (const entry of this.lazyProviders.values()){
            entry.cleanups.forEach((c)=>c());
            entry.cleanups = [];
        }
    }
    /**
   * (Re)build lazily provided shortcuts whose prefix is compatible with `path`
   * (the path is a prefix of theirs, or theirs of it). With an empty path every
   * provider resolves, which the fuzzy palette needs. Returns when done; old
   * provided entries are replaced.
   */ resolveLazy(path) {
        for (const entry of this.lazyProviders.values()){
            const n = Math.min(path.length, entry.prefix.length);
            const compatible = path.slice(0, n).every((k, i)=>k === entry.prefix[i]);
            if (!compatible) continue;
            entry.cleanups.forEach((c)=>c());
            entry.cleanups = entry.provider().map((param)=>{
                let { id, shortcut } = param;
                return this.register(id, shortcut);
            });
        }
    }
    /**
   * Detect the active context by walking up the DOM from document.activeElement
   * looking for a [data-context] attribute.
   */ getActiveContext() {
        if (typeof document === "undefined") return "global";
        let el = document.activeElement;
        while(el){
            const ctx = el.getAttribute("data-context");
            if (ctx) return ctx;
            el = el.parentElement;
        }
        return "global";
    }
    dispatch(event) {
        // Skip IME composition for single-character shortcuts
        if (event.isComposing) return;
        // Skip if target is an input/textarea/select/contenteditable
        // (unless the shortcut explicitly uses a modifier)
        const hasModifier = event.metaKey || event.ctrlKey || event.altKey;
        if (!hasModifier && isInputElement(event.target)) return;
        const activeContext = this.getActiveContext();
        for (const shortcut of this.shortcuts.values()){
            if (shortcut.key === undefined && shortcut.code === undefined) continue;
            // Context check: global shortcuts fire everywhere (except inside terminal unless
            // the shortcut is terminal-scoped). Terminal-scoped shortcuts only fire in terminal.
            if (shortcut.context === "terminal" && activeContext !== "terminal") continue;
            if (activeContext === "terminal" && shortcut.context !== "terminal" && shortcut.context !== "global" && shortcut.context !== "cockpit") continue;
            if (shortcut.context !== "global" && shortcut.context !== activeContext) continue;
            if (!keyMatches(event, shortcut)) continue;
            event.preventDefault();
            shortcut.action();
            return; // first match wins
        }
    }
    /** Tear down the global listener (useful in tests). */ destroy() {
        if (typeof document !== "undefined") {
            document.removeEventListener("keydown", this.bound);
        }
    }
    constructor(){
        this.shortcuts = new Map();
        this.lazyProviders = new Map();
        this.bound = this.dispatch.bind(this);
        if (typeof document !== "undefined") {
            document.addEventListener("keydown", this.bound);
        }
    }
}
function keyMatches(event, shortcut) {
    const keyOk = shortcut.key !== undefined && event.key === shortcut.key;
    const codeOk = shortcut.code !== undefined && event.code === shortcut.code;
    if (!keyOk && !codeOk) return false;
    var _shortcut_modifiers;
    const m = (_shortcut_modifiers = shortcut.modifiers) !== null && _shortcut_modifiers !== void 0 ? _shortcut_modifiers : {};
    if (!!m.meta !== event.metaKey) return false;
    if (!!m.ctrl !== event.ctrlKey) return false;
    if (!!m.shift !== event.shiftKey) return false;
    if (!!m.alt !== event.altKey) return false;
    return true;
}
function isInputElement(target) {
    if (!(target instanceof Element)) return false;
    const el = target;
    const tag = el.tagName.toLowerCase();
    if (tag === "input" || tag === "textarea" || tag === "select") return true;
    if (el instanceof HTMLElement && el.isContentEditable) return true;
    return false;
}
/** Singleton registry — import this in components and hooks. */ const registry = new ShortcutRegistry();


/***/ }),

/***/ 17073:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   o: () => (/* binding */ recordSpan),
/* harmony export */   y: () => (/* binding */ getSessionSpansJson)
/* harmony export */ });
// Bounded in-memory record of ended browser spans, read by the diagnostic
// bundle export. No OTel imports so components can use it without triggering
// otel-init.ts's module-level SDK setup.
const MAX_SPANS = 2000;
const spans = [];
function recordSpan(span) {
    spans.push(span);
    if (spans.length > MAX_SPANS) spans.shift();
}
/** Spans tagged with sessionUuid, JSON-encoded for the export request. */ function getSessionSpansJson(sessionUuid) {
    const matching = spans.filter((s)=>s.attributes["session.uuid"] === sessionUuid);
    return new TextEncoder().encode(JSON.stringify(matching));
}


/***/ }),

/***/ 21777:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   w: () => (/* binding */ useShortcut)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _shortcutRegistry__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(8569);
/* __next_internal_client_entry_do_not_use__ useShortcut auto */ 

/**
 * useShortcut — registers a keyboard shortcut on mount, deregisters on unmount.
 *
 * @param id     Unique string identifier for this shortcut (used for deregistration and ? overlay)
 * @param shortcut  Shortcut definition (key, modifiers, label, context, action)
 *
 * The action callback is re-registered whenever it changes (identity comparison),
 * so wrap it in useCallback to avoid unnecessary re-registrations.
 *
 * Pass `enabled: false` from a component that can be mounted many times at once (one per pane)
 * so only one instance owns the id; otherwise each mount overwrites the last and the first
 * unmount deletes the survivors' registration.
 */ function useShortcut(id, shortcut) {
    let enabled = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : true;
    var _shortcut_sequence;
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!enabled) return;
        const cleanup = _shortcutRegistry__WEBPACK_IMPORTED_MODULE_1__/* .registry */ .u5.register(id, shortcut);
        return cleanup;
    // Re-register when shortcut identity changes. Callers should useCallback their action.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        id,
        enabled,
        shortcut.key,
        shortcut.code,
        shortcut.modifiers,
        shortcut.context,
        shortcut.label,
        shortcut.action,
        (_shortcut_sequence = shortcut.sequence) === null || _shortcut_sequence === void 0 ? void 0 : _shortcut_sequence.join("\u0000"),
        shortcut.group,
        shortcut.hidden
    ]);
}


/***/ }),

/***/ 31205:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   w: () => (/* binding */ formatPauseReason)
/* harmony export */ });
function formatPauseReason(reason) {
    switch(reason){
        case "manual":
            return "Paused manually";
        case "auto:inactivity":
            return "Paused automatically — no recent activity";
        case "auto:session_limit":
            return "Paused automatically — too many active sessions";
        case "auto:resource":
            return "Paused automatically — resource pressure";
        default:
            return "Paused";
    }
}


/***/ }),

/***/ 34892:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  s: () => (/* binding */ ExportBundleModal)
});

// UNUSED EXPORTS: CONSENT_TEXT

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ./src/components/ui/Modal.tsx + 1 modules
var Modal = __webpack_require__(79898);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/file.js
var file = __webpack_require__(1844);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/message.js
var message = __webpack_require__(15033);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/service.js
var service = __webpack_require__(95765);
;// ./src/gen/session/v1/rum_bundle_pb.ts
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/rum_bundle.proto (package session.v1, syntax proto3)
/* eslint-disable */ 
/**
 * Describes the file session/v1/rum_bundle.proto.
 */ const file_session_v1_rum_bundle = /*@__PURE__*/ (0,file/* fileDesc */.w)("ChtzZXNzaW9uL3YxL3J1bV9idW5kbGUucHJvdG8SCnNlc3Npb24udjEiawoaRXhwb3J0U2Vzc2lvbkJ1bmRsZVJlcXVlc3QSFAoMc2Vzc2lvbl91dWlkGAEgASgJEhwKFG1heF9zY3JvbGxiYWNrX2xpbmVzGAIgASgFEhkKEWNsaWVudF9zcGFuc19qc29uGAMgASgMIj8KC0J1bmRsZUNodW5rEgwKBGRhdGEYASABKAwSEwoLY2h1bmtfaW5kZXgYAiABKAUSDQoFZmluYWwYAyABKAgiXwoYU2F2ZVNlc3Npb25CdW5kbGVSZXF1ZXN0EhQKDHNlc3Npb25fdXVpZBgBIAEoCRISCgpvdXRwdXRfZGlyGAIgASgJEhkKEWNsaWVudF9zcGFuc19qc29uGAMgASgMIl0KGVNhdmVTZXNzaW9uQnVuZGxlUmVzcG9uc2USEQoJZmlsZV9wYXRoGAEgASgJEhcKD2ZpbGVfc2l6ZV9ieXRlcxgCIAEoAxIUCgxjb250ZW50X2hhc2gYAyABKAky1gEKFFNlc3Npb25CdW5kbGVTZXJ2aWNlEloKE0V4cG9ydFNlc3Npb25CdW5kbGUSJi5zZXNzaW9uLnYxLkV4cG9ydFNlc3Npb25CdW5kbGVSZXF1ZXN0Ghcuc2Vzc2lvbi52MS5CdW5kbGVDaHVuayIAMAESYgoRU2F2ZVNlc3Npb25CdW5kbGUSJC5zZXNzaW9uLnYxLlNhdmVTZXNzaW9uQnVuZGxlUmVxdWVzdBolLnNlc3Npb24udjEuU2F2ZVNlc3Npb25CdW5kbGVSZXNwb25zZSIAQq4BCg5jb20uc2Vzc2lvbi52MUIOUnVtQnVuZGxlUHJvdG9QAVpDZ2l0aHViLmNvbS90c3RhcGxlci9zdGFwbGVyLXNxdWFkL2dlbi9wcm90by9nby9zZXNzaW9uL3YxO3Nlc3Npb252MaICA1NYWKoCClNlc3Npb24uVjHKAgpTZXNzaW9uXFYx4gIWU2Vzc2lvblxWMVxHUEJNZXRhZGF0YeoCC1Nlc3Npb246OlYxYgZwcm90bzM");
/**
 * Describes the message session.v1.ExportSessionBundleRequest.
 * Use `create(ExportSessionBundleRequestSchema)` to create a new message.
 */ const ExportSessionBundleRequestSchema = /*@__PURE__*/ (0,message/* messageDesc */.Z)(file_session_v1_rum_bundle, 0);
/**
 * Describes the message session.v1.BundleChunk.
 * Use `create(BundleChunkSchema)` to create a new message.
 */ const BundleChunkSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_rum_bundle, 1)));
/**
 * Describes the message session.v1.SaveSessionBundleRequest.
 * Use `create(SaveSessionBundleRequestSchema)` to create a new message.
 */ const SaveSessionBundleRequestSchema = /*@__PURE__*/ (0,message/* messageDesc */.Z)(file_session_v1_rum_bundle, 2);
/**
 * Describes the message session.v1.SaveSessionBundleResponse.
 * Use `create(SaveSessionBundleResponseSchema)` to create a new message.
 */ const SaveSessionBundleResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_rum_bundle, 3)));
/**
 * SessionBundleService provides per-session diagnostic bundle export (RUM —
 * Real User Monitoring). Supports two delivery modes:
 *   - ExportSessionBundle: server-streaming, chunks the ZIP to the browser
 *     (Approach A from the implementation plan; auth middleware applies
 *     automatically via ConnectOptions; consistent with WatchSessions /
 *     StreamTerminal streaming pattern).
 *   - SaveSessionBundle: unary, writes the ZIP to a server-side path and
 *     returns the file path + SHA-256 content hash for backlog hand-off.
 *
 * @generated from service session.v1.SessionBundleService
 */ const SessionBundleService = /*@__PURE__*/ (0,service/* serviceDesc */.Z)(file_session_v1_rum_bundle, 0);

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
// EXTERNAL MODULE: ./src/lib/telemetry/span-buffer.ts
var span_buffer = __webpack_require__(17073);
;// ./src/lib/hooks/useExportBundle.ts
/* __next_internal_client_entry_do_not_use__ useExportBundle auto */ // +feature: ui:bundle-export






const ZIP_MIME_TYPE = "application/zip";
function slugifyTitle(title) {
    return title.toLowerCase().replace(/\s+/g, "-").replace(/[^a-z0-9-]/g, "");
}
function formatTimestamp(date) {
    const pad = (n)=>String(n).padStart(2, "0");
    const ymd = "".concat(date.getFullYear()).concat(pad(date.getMonth() + 1)).concat(pad(date.getDate()));
    const hms = "".concat(pad(date.getHours())).concat(pad(date.getMinutes())).concat(pad(date.getSeconds()));
    return "".concat(ymd, "-").concat(hms);
}
function buildFilename(sessionTitle) {
    return "".concat(slugifyTitle(sessionTitle), "-").concat(formatTimestamp(new Date()), ".zip");
}
function triggerBrowserDownload(blob, filename) {
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(()=>URL.revokeObjectURL(url), 1000);
}
async function deliverBlob(blob, filename) {
    var _navigator_canShare, _navigator;
    const file = new File([
        blob
    ], filename, {
        type: ZIP_MIME_TYPE
    });
    if (typeof navigator !== "undefined" && ((_navigator_canShare = (_navigator = navigator).canShare) === null || _navigator_canShare === void 0 ? void 0 : _navigator_canShare.call(_navigator, {
        files: [
            file
        ]
    }))) {
        try {
            await navigator.share({
                files: [
                    file
                ]
            });
        } catch (err) {
            const name = err instanceof DOMException ? err.name : "";
            if (name === "AbortError") return; // user dismissed the share sheet
            if (name !== "NotAllowedError") throw err;
            triggerBrowserDownload(blob, filename); // user activation expired during the stream
        }
    } else {
        triggerBrowserDownload(blob, filename);
    }
}
// kibitzer:ignore long-function -- standard React hook: two useCallback handlers + setup boilerplate cannot be split further without sacrificing readability
function useExportBundle() {
    const [loading, setLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    // Initialize client inside useEffect to avoid SSR issues (matching useExportRules pattern).
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(SessionBundleService, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const download = (0,react.useCallback)(async (sessionUuid, sessionTitle)=>{
        if (!clientRef.current) return false;
        setLoading(true);
        setError(null);
        try {
            const req = (0,create/* create */.v)(ExportSessionBundleRequestSchema, {
                sessionUuid,
                maxScrollbackLines: 0,
                clientSpansJson: (0,span_buffer/* getSessionSpansJson */.y)(sessionUuid)
            });
            const chunks = [];
            for await (const chunk of clientRef.current.exportSessionBundle(req)){
                if (chunk.data && chunk.data.length > 0) {
                    chunks.push(new Uint8Array(chunk.data));
                }
            }
            const blob = new Blob(chunks, {
                type: ZIP_MIME_TYPE
            });
            await deliverBlob(blob, buildFilename(sessionTitle));
            return true;
        } catch (err) {
            setError(err instanceof Error ? err.message : "Export failed");
            return false;
        } finally{
            setLoading(false);
        }
    }, []);
    const saveOnServer = (0,react.useCallback)(async (sessionUuid)=>{
        if (!clientRef.current) throw new Error("Client not initialized");
        setLoading(true);
        setError(null);
        try {
            const req = (0,create/* create */.v)(SaveSessionBundleRequestSchema, {
                sessionUuid,
                outputDir: "",
                clientSpansJson: (0,span_buffer/* getSessionSpansJson */.y)(sessionUuid)
            });
            return await clientRef.current.saveSessionBundle(req);
        } catch (err) {
            setError(err instanceof Error ? err.message : "Save failed");
            throw err;
        } finally{
            setLoading(false);
        }
    }, []);
    return {
        download,
        saveOnServer,
        loading,
        error
    };
}

// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
// EXTERNAL MODULE: ./src/lib/contexts/NotificationContext.tsx + 12 modules
var NotificationContext = __webpack_require__(62275);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FExportBundleModal.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xd2oxM3h2MCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgbWFyZ2luOiB2YXIoLS1fMTllejJyejMxKSAwOwogIGJvcmRlcjogbm9uZTsKICBwYWRkaW5nOiAwOwp9Ci5fMXdqMTN4djEgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwogIG1pbi1oZWlnaHQ6IDQ0cHg7CiAgY3Vyc29yOiBwb2ludGVyOwp9Ci5fMXdqMTN4djIgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGZsZXgtc3RhcnQ7CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ6KTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejMwKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzgpOwp9Ci5fMXdqMTN4djMgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBqdXN0aWZ5LWNvbnRlbnQ6IHNwYWNlLWJldHdlZW47CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ6KTsKICBtYXJnaW4tdG9wOiB2YXIoLS1fMTllejJyejMxKTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejMwKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzgpOwp9Ci5fMXdqMTN4djQgewogIG1hcmdpbi10b3A6IHZhcigtLV8xOWV6MnJ6MzEpOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FExportBundleModal_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xd2oxM3h2MCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgbWFyZ2luOiB2YXIoLS1fMTllejJyejMxKSAwOwogIGJvcmRlcjogbm9uZTsKICBwYWRkaW5nOiAwOwp9Ci5fMXdqMTN4djEgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwogIG1pbi1oZWlnaHQ6IDQ0cHg7CiAgY3Vyc29yOiBwb2ludGVyOwp9Ci5fMXdqMTN4djIgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGZsZXgtc3RhcnQ7CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ6KTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejMwKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzgpOwp9Ci5fMXdqMTN4djMgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBqdXN0aWZ5LWNvbnRlbnQ6IHNwYWNlLWJldHdlZW47CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ6KTsKICBtYXJnaW4tdG9wOiB2YXIoLS1fMTllejJyejMxKTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejMwKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzgpOwp9Ci5fMXdqMTN4djQgewogIG1hcmdpbi10b3A6IHZhcigtLV8xOWV6MnJ6MzEpOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwp9_22_7D = __webpack_require__(75641);
;// ./src/components/sessions/ExportBundleModal.css.ts


var consent = '_1wj13xv2';
var errorBanner = '_1wj13xv3';
var radioGroup = '_1wj13xv0';
var radioLabel = '_1wj13xv1';
var resultBox = '_1wj13xv4';
;// ./src/components/sessions/ExportBundleModal.tsx
/* __next_internal_client_entry_do_not_use__ CONSENT_TEXT,ExportBundleModal auto */ 
// +feature: ui:bundle-export







const CONSENT_TEXT = "This bundle may contain raw terminal output including typed credentials. Do not share it with untrusted parties.";
function ymd(date) {
    const pad = (n)=>String(n).padStart(2, "0");
    return "".concat(date.getFullYear()).concat(pad(date.getMonth() + 1)).concat(pad(date.getDate()));
}
function basename(path) {
    var _path_split_pop;
    return (_path_split_pop = path.split("/").pop()) !== null && _path_split_pop !== void 0 ? _path_split_pop : path;
}
function ExportBundleModal(param) {
    let { open, onOpenChange, sessionId, sessionTitle, repoPath } = param;
    const { download, saveOnServer, loading, error } = useExportBundle();
    const { createBacklogItem } = (0,useBacklogService/* useBacklogService */.gG)();
    const { showActionToast } = (0,NotificationContext/* useNotifications */.E$)();
    const [mode, setMode] = (0,react.useState)(()=> true && window.innerWidth < 640 ? "save" : "download");
    const [consented, setConsented] = (0,react.useState)(false);
    const [saved, setSaved] = (0,react.useState)(null);
    const [backlogItemId, setBacklogItemId] = (0,react.useState)(null);
    const [creating, setCreating] = (0,react.useState)(false);
    const [failed, setFailed] = (0,react.useState)(false);
    const toastKey = "bundle-export-".concat(sessionId);
    const handleExport = async ()=>{
        setFailed(false);
        setSaved(null);
        setBacklogItemId(null);
        try {
            if (mode === "download") {
                const ok = await download(sessionId, sessionTitle);
                if (ok) showActionToast("Bundle downloaded.", "success", toastKey);
                else throw new Error("Export failed");
            } else {
                setSaved(await saveOnServer(sessionId));
                showActionToast("Bundle saved to server.", "success", toastKey);
            }
        } catch (e) {
            setFailed(true);
            showActionToast("Bundle export failed.", "error", toastKey);
        }
    };
    const handleCreateBacklogItem = async ()=>{
        if (!saved) return;
        setCreating(true);
        const filename = basename(saved.filePath);
        let result = null;
        try {
            result = await createBacklogItem({
                title: "Diagnostic bundle: ".concat(sessionTitle, " (").concat(ymd(new Date()), ")"),
                notes: "Bundle: ".concat(filename, " (SHA-256: ").concat(saved.contentHash, ")"),
                repoPath
            });
        } finally{
            setCreating(false);
        }
        if (result) {
            setBacklogItemId(result.item.id);
            showActionToast("Backlog item created.", "success", toastKey);
        } else {
            showActionToast("Failed to create backlog item.", "error", toastKey);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* Modal */.aF, {
        open: open,
        onOpenChange: onOpenChange,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(Modal/* ModalContent */.$m, {
            "data-testid": "export-bundle-modal",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* ModalTitle */.wt, {
                    children: "Export Diagnostics"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* ModalDescription */.mw, {
                    children: "Package this session's traces, profiles and state into one zip."
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("fieldset", {
                    className: radioGroup,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("legend", {
                            children: "Delivery"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                            className: radioLabel,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                    type: "radio",
                                    name: "bundle-mode",
                                    checked: mode === "download",
                                    onChange: ()=>setMode("download")
                                }),
                                "Download .zip"
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                            className: radioLabel,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                    type: "radio",
                                    name: "bundle-mode",
                                    checked: mode === "save",
                                    onChange: ()=>setMode("save")
                                }),
                                "Save on server"
                            ]
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                    className: consent,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            type: "checkbox",
                            "aria-required": "true",
                            checked: consented,
                            onChange: (e)=>setConsented(e.target.checked),
                            "data-testid": "export-bundle-consent"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            children: CONSENT_TEXT
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    role: "status",
                    "aria-live": "polite",
                    children: loading ? "Assembling bundle…" : ""
                }),
                (failed || error) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    role: "alert",
                    className: errorBanner,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            children: error !== null && error !== void 0 ? error : "Export failed"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            onClick: handleExport,
                            disabled: loading,
                            children: "Retry"
                        })
                    ]
                }),
                saved && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: resultBox,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            children: [
                                "Saved: ",
                                basename(saved.filePath)
                            ]
                        }),
                        backlogItemId ? /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                            href: "/backlog?item=".concat(backlogItemId),
                            children: "View backlog item →"
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            onClick: handleCreateBacklogItem,
                            disabled: creating,
                            "data-testid": "create-backlog-from-bundle",
                            children: "Create backlog item →"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)(Modal/* ModalFooter */.jl, {
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            onClick: ()=>onOpenChange(false),
                            children: "Close"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            onClick: handleExport,
                            disabled: !consented || loading,
                            "data-testid": "export-bundle-submit",
                            children: loading ? "Assembling bundle…" : "Export"
                        })
                    ]
                })
            ]
        })
    });
}


/***/ }),

/***/ 55765:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   ViewportProvider: () => (/* binding */ ViewportProvider),
/* harmony export */   k: () => (/* binding */ useViewport)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _lib_terminal_mobileDebug__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(99050);
/* __next_internal_client_entry_do_not_use__ useViewport,ViewportProvider auto */ 


const ViewportContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)({
    isMobile: true,
    isFoldable: false,
    isInnerScreen: false,
    isLandscape: false,
    hasFinePointer: false,
    isVirtualKeyboardOpen: false
});
function useViewport() {
    return (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(ViewportContext);
}
function ViewportProvider(param) {
    let { children } = param;
    const [viewport, setViewport] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)({
        isMobile: true,
        isFoldable: false,
        isInnerScreen: false,
        isLandscape: false,
        hasFinePointer: false,
        isVirtualKeyboardOpen: false
    });
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        // Set CSS variables from visualViewport (keyboard height, viewport height)
        const vv = window.visualViewport;
        if (!vv) return;
        const logViewport = (event)=>()=>{
                if (!_lib_terminal_mobileDebug__WEBPACK_IMPORTED_MODULE_2__/* .mobileDebug */ .Uo.enabled()) return; // skip building the payload while the flag is off
                _lib_terminal_mobileDebug__WEBPACK_IMPORTED_MODULE_2__/* .mobileDebug */ .Uo.log('viewport', {
                    event,
                    height: vv.height,
                    offsetTop: vv.offsetTop
                });
            };
        const onResize = logViewport('resize');
        const onScroll = logViewport('scroll');
        const update = ()=>{
            requestAnimationFrame(()=>{
                // Must listen to both resize AND scroll events on iOS Safari —
                // scroll fires during keyboard transitions alongside resize.
                const kb = Math.max(0, window.innerHeight - vv.height - vv.offsetTop);
                document.documentElement.style.setProperty('--keyboard-height', "".concat(kb, "px"));
                document.documentElement.style.setProperty('--viewport-height', "".concat(vv.height, "px"));
                // Small threshold avoids false positives from browser chrome/rounding jitter.
                setViewport((prev)=>({
                        ...prev,
                        isVirtualKeyboardOpen: kb > 100
                    }));
            });
        };
        vv.addEventListener('resize', onResize);
        vv.addEventListener('scroll', onScroll);
        vv.addEventListener('resize', update);
        vv.addEventListener('scroll', update);
        update();
        return ()=>{
            vv.removeEventListener('resize', onResize);
            vv.removeEventListener('scroll', onScroll);
            vv.removeEventListener('resize', update);
            vv.removeEventListener('scroll', update);
        };
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        // Detects a real mouse/trackpad attached to an otherwise-touch device
        // (Bluetooth/USB mouse on Android Chrome or iPadOS Safari) — updates live.
        // Must use any-hover/any-pointer, not hover/pointer: the non-"any" features
        // reflect the primary input mechanism, which stays touch/coarse on most
        // mobile browsers even once a mouse is attached.
        const mq = window.matchMedia('(any-hover: hover) and (any-pointer: fine)');
        const update = ()=>{
            setViewport((prev)=>({
                    ...prev,
                    hasFinePointer: mq.matches
                }));
        };
        mq.addEventListener('change', update);
        update();
        return ()=>mq.removeEventListener('change', update);
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        // Track breakpoint state for responsive rendering
        const update = ()=>{
            const w = window.innerWidth;
            setViewport((prev)=>({
                    ...prev,
                    isMobile: w < 600,
                    isFoldable: w >= 600 && w < 900,
                    isInnerScreen: w >= 900,
                    isLandscape: w < 900 && w > window.innerHeight
                }));
        };
        update();
        window.addEventListener('resize', update);
        return ()=>window.removeEventListener('resize', update);
    }, []);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(ViewportContext.Provider, {
        value: viewport,
        children: children
    });
}


/***/ }),

/***/ 61053:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  h: () => (/* binding */ ResumeSessionModal)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/hooks/useFocusTrap.ts
var useFocusTrap = __webpack_require__(63991);
;// ./src/utils/sessionNameUtils.ts
/**
 * Generate a unique session name by appending a numeric suffix if needed.
 * Strips any existing "(N)" suffix before checking for conflicts.
 */ function generateUniqueName(baseName, existingNames) {
    if (!baseName || !baseName.trim()) return baseName;
    const existingSet = new Set(existingNames);
    // Strip trailing " (N)" suffix to get the root name
    const stripped = baseName.replace(/\s*\(\d+\)$/, "").trim();
    if (!existingSet.has(stripped)) return stripped;
    // If the original name (with suffix) doesn't conflict, keep it
    if (baseName !== stripped && !existingSet.has(baseName)) return baseName;
    let counter = 2;
    while(existingSet.has("".concat(stripped, " (").concat(counter, ")")))counter++;
    return "".concat(stripped, " (").concat(counter, ")");
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FmodalChrome.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VVTW-jMBC951fMpVKQ1pGhbZqQS__GnlYONuAWbNZ2UpJV__sKG1JwgGSl3cNKETH-ePPmzfPw-s5OqSIl01BvxbqkGH4tAFIlSzsAkBVJuDnFgHcLgM8FgJH-UuiWPhevPlzowxlFhE6lKmM3LIhh35cRrupgdyve-FnsH7ywWTkOkT1eSc0NlyKGlNeM7ixw1YYpWGraoeJZ3o330hhZdi8kec-UPAgaw5GoJUI_wi07R-pMLAPKdVWQUwxpwepmghQ8E4gbVuoYEiYMU83020Ebnp5QIoVhwvSXzogLyuoYQoxtTCJ4SRzrrjx4FWlgRLPdV4aPNsNpgtvAZaMoU0gRyg86hjCqLM2KUMpF1mZZkhp9cGryGJ6bqjRz7fsWP3Q7cuZU2uBjPpp7848oVyxx7BNZHErhWNRI54TKjxgwNCFg3TxUtidL_A3a3-oxGBUgbJauBHhyJe4yiZ6q2j3CdTvqCdBVNaxq0LLg1FPrLeghP1vkkqiMi4YwhgbWSpVKYZDmZxZDp5Sd-mjFWbsaJrKQyq8I7sdYD2N42GHLfhQn7OO8TBmdC83MlY3RKOK_M_OlPMOAj4McNp6Vx0n2DT1fyCvbe8E3gW9Kb0MZzHGPAu_SPG2GlyYhRbIMMX4ABJFi5bi609clI9W8YFvXYb_84m2mwZUxvR15MGvTgTMvYYkNO-nJOUaJDwpQcMEuTSVcPfcC7W2gK8XGdHFsryxpxWWCfkVFtvMPT0fnvqrJXTYsp5X7OXCokILd9uLLnNWiczDmg9tCz5U-cwkclG4yqCTvLq_9vrZtxDsjBjrFuTwydZda3sFUJgeNjlzzfcEsgDyYxggxRBMX2undbkMyTW1bs9-xCzCd4GIzqohiwsx209tthd3RVv7DUtI_KOWmL8HoDj6E_svF_g3PZClKtwoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FmodalChrome_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VVTW_jMBC951fMpVKQ1pGhbZqQS_GnlYONuAWbNZ2UpJV_sKG1JwgGSl3cNKETH_ePPmzfPw_s5OqSIl01BvxbqkGH4tAFIlSzsAkBVJuDnFgHcLgM8FgJH_UuiWPhevPlzowxlFhE6lKmM3LIhh35cRrupgdyve_FnsH7ywWTkOkT1eSc0NlyKGlNeM7ixw1YYpWGraoeJZ3o330hhZdi8kec_UPAgaw5GoJUI_wi07R_pMLAPKdVWQUwxpwepmghQ8E4gbVuoYEiYMU83020Ebnp5QIoVhwvSXzogLyuoYQoxtTCJ4SRzrrjx4FWlgRLPdV4aPNsNpgtvAZaMoU0gRyg86hjCqLM2KUMpF1mZZkhp9cGryGJ6bqjRz7fsWP3Q7cuZU2uBjPpp7848oVyxx7BNZHErhWNRI54TKjxgwNCFg3TxUtidL_A3a3_oxGBUgbJauBHhyJe4yiZ6q2j3CdTvqCdBVNaxq0LLg1FPrLeghP1vkkqiMi4YwhgbWSpVKYZDmZxZDp5Sd_mjFWbsaJrKQyq8I7sdYD2N42GHLfhQn7OO8TBmdC83MlY3RKOK_M_OlPMOAj4McNp6Vx0n2DT1fyCvbe8E3gW9Kb0MZzHGPAu_SPG2GlyYhRbIMMX4ABJFi5bi609clI9W8YFvXYb_84m2mwZUxvR15MGvTgTMvYYkNO_nJOUaJDwpQcMEuTSVcPfcC7W2gK8XGdHFsryxpxWWCfkVFtvMPT0fnvqrJXTYsp5X7OXCokILd9uLLnNWiczDmg9tCz5U_cwkclG4yqCTvLq_9vrZtxDsjBjrFuTwydZda3sFUJgeNjlzzfcEsgDyYxggxRBMX2undbkMyTW1bs9_xCzCd4GIzqohiwsx209tthd3RVv7DUtI_KOWmL8HoDj6E_svF_g3PZClKtwoAAA_22_7D = __webpack_require__(6579);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FResumeSessionModal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81XXW-bMBR976-4L5OCVCKbpG3m_pI9TQ52wI2xmTEN6bT_PtkQahJIUNdpU6U2Bd-vc889vlmyl2NW7hD8vAMoKWNCZQSSddk83wHoV252Uh_iIwFaW-2e7SRvCODnu193y9YYe-OCmkyoeKut1QWBBDkX_ZnEn2GiKiU9EthKne6feysCCBA8lg0gH0IrG1fijRPAqzYT_-jARZZbAo_IH0u11IbAKzWLOP6Ov_K3xLzhyL2yvLGxNVRVO20KAnVZcpPSiruXklvLTVyVNPXVouXDINeVz_UgmM0JYIS-PIfYYFQ2gDuAttowbgjgsoFKS8HOstlH76diQ5moK-Lq9E9pus-MrhW7qIFGk_Wh6ByhLhVfrbBCKwJUSkDLpALelbzVjTvuC-iy2epBzWSn07rylevaSqE4AaUVD_IfzaeIev85ZfrQthLBqmzAZFu6QPeAcXIPyXp1D2jp-hNEJaWkKc-1ZNz44KNB1qHR-gaVPI3czzlQSQvUaIQmumDZg2NZH_VhBskfh5m5SXFeM1oS2LTBzxzgoYMn76CfsIB1G0e65P8h3eom6d6L-ifUevoItTZDGfSg3wSumMbtRxT266zms87MFb3aVC5QqYWy3MxtwobkTsx9fdOlqBCMr-Nsdn_jg3Gcdr-HBO-N6bgxlSJTsbC8qAik_FSBd9Dh0KPv5ridj2sps2jOSGRXRmIOfHQWfJvoFqXzEOBtO-2Xfb81g-MKlQ7J69lyusQ69ck7w9P_V9QAT7MaJ3No_dDem2F5XVSnAHGfygSp51Lnpa6s2B3jVCvLlQ1fzWlsOquxuAU_WCiqlEq-wEPNYcEd0TXgfW04sc1tJr6WMNVJaRrhRx-OD8Ktw2tvvG-XMp6MCM2QVbuxey88kF2RCSYMT9sGpFrWhRoOe-8jv31xTsMgpsUdT8wMQCFU3A3HU6cxPuMqN0LthxW-TAdoET2tyQRywRhXfZ_fX3ApRVmJyg9lLiz366e_CFsV7aPth2PseNPv492IWV1OS91LNDo-F3Piy-WK9UC3ZOjzkGd5oDCPyUlJby0Rf6LUI1vvJ16RspUCorRdECYqupWcRZ8r-bJ33K5D7iuIPfqvIGHmSjuNkPrAWWBcfKgjf2E9-Xzsi49gr85F2X-U1PJvixiXTTQIQFMrXvlYhHEPaGj-gb79Bg0LcIJcDwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FResumeSessionModal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81XXW_bMBR976_4L5OCVCKbpG3m_pI9TQ52wI2xmTEN6bT_PtkQahJIUNdpU6U2Bd_vc889vlmyl2NW7hD8vAMoKWNCZQSSddk83wHoV252Uh_iIwFaW_2e7SRvCODnu193y9YYe_OCmkyoeKut1QWBBDkX_ZnEn2GiKiU9EthKne6feysCCBA8lg0gH0IrG1fijRPAqzYT_jARZZbAo_IH0u11IbAKzWLOP6Ov_K3xLzhyL2yvLGxNVRVO20KAnVZcpPSiruXklvLTVyVNPXVouXDINeVz_UgmM0JYIS_PIfYYFQ2gDuAttowbgjgsoFKS8HOstlH76diQ5moK_Lq9E9pus_MrhW7qIFGk_Wh6ByhLhVfrbBCKwJUSkDLpALelbzVjTvuC_iy2epBzWSn07rylevaSqE4AaUVD_IfzaeIev85ZfrQthLBqmzAZFu6QPeAcXIPyXp1D2jp_hNEJaWkKc_1ZNz44KNB1qHR_gaVPI3czzlQSQvUaIQmumDZg2NZH_VhBskfh5m5SXFeM1oS2LTBzxzgoYMn76CfsIB1G0e65P8h3eom6d6L_ifUevoItTZDGfSg3wSumMbtRxT266zms87MFb3aVC5QqYWy3MxtwobkTsx9fdOlqBCMr_Nsdn_jg3Gcdr_HBO_N6bgxlSJTsbC8qAik_FSBd9Dh0KPv5ridj2sps2jOSGRXRmIOfHQWfJvoFqXzEOBtO_2Xfb81g_MKlQ7J69lyusQ69ck7w9P_V9QAT7MaJ3No_dDem2F5XVSnAHGfygSp51Lnpa6s2B3jVCvLlQ1fzWlsOquxuAU_WCiqlEq_wEPNYcEd0TXgfW04sc1tJr6WMNVJaRrhRx_OD8Ktw2tvvG_XMp6MCM2QVbuxey88kF2RCSYMT9sGpFrWhRoOe_8jv31xTsMgpsUdT8wMQCFU3A3HU6cxPuMqN0LthxW_TAdoET2tyQRywRhXfZ_fX3ApRVmJyg9lLiz366e_CFsV7aPth2PseNPv492IWV1OS91LNDo_F3Piy_WK9UC3ZOjzkGd5oDCPyUlJby0Rf6LUI1vvJ16RspUCorRdECYqupWcRZ8r_bJ33K5D7iuIPfqvIGHmSjuNkPrAWWBcfKgjf2E9_Xzsi49gr85F2X_U1PJvixiXTTQIQFMrXvlYhHEPaGj_gb79Bg0LcIJcDwAA_22_7D = __webpack_require__(28200);
;// ./src/components/sessions/ResumeSessionModal.css.ts



var addTagButton = 'djygpf8';
var body = 'djygpf0';
var cancelButton = 'djygpfl';
var conflictHint = 'djygpf4';
var contextGrid = 'djygpfg';
var contextLabel = 'djygpfi';
var contextRow = 'djygpfh';
var contextSection = 'djygpff';
var contextValue = 'djygpfj';
var emptyTags = 'djygpfd';
var fieldGroup = 'djygpf1';
var fieldLabel = 'djygpf2';
var footer = 'djygpfk';
var header = 'x9n6md4';
var modal = 'x9n6md3';
var overlay = 'x9n6md2';
var removeTagButton = 'djygpfc';
var resumeButton = 'djygpfm';
var subtitle = 'x9n6md6';
var ResumeSessionModal_css_tagError = 'djygpfe';
var ResumeSessionModal_css_tagInput = 'djygpf7';
var tagInputRow = 'djygpf6';
var tagItem = 'djygpfa';
var tagText = 'djygpfb';
var tagsList = 'djygpf9';
var tagsSection = 'djygpf5';
var ResumeSessionModal_css_title = 'x9n6md5';
var titleInput = 'djygpf3';
;// ./src/components/sessions/ResumeSessionModal.tsx
/* __next_internal_client_entry_do_not_use__ ResumeSessionModal auto */ 




function ResumeSessionModal(param) {
    let { session, sessions, onConfirm, onCancel, triggerRef } = param;
    const modalRef = (0,react.useRef)(null);
    const titleInputRef = (0,react.useRef)(null);
    (0,useFocusTrap/* useFocusTrap */.r)(modalRef, true, triggerRef);
    // Compute conflict detection once at mount
    const [initialState] = (0,react.useState)(()=>{
        const otherNames = sessions.filter((s)=>s.id !== session.id).map((s)=>s.title);
        const hasConflict = otherNames.includes(session.title);
        const suggestedTitle = hasConflict ? generateUniqueName(session.title, otherNames) : session.title;
        return {
            hasConflict,
            originalTitle: session.title,
            suggestedTitle
        };
    });
    const [title, setTitle] = (0,react.useState)(initialState.suggestedTitle);
    const [tags, setTags] = (0,react.useState)([
        ...session.tags || []
    ]);
    const [tagInput, setTagInput] = (0,react.useState)("");
    const [tagError, setTagError] = (0,react.useState)(null);
    const [isSubmitting, setIsSubmitting] = (0,react.useState)(false);
    // Auto-focus the title input on mount
    (0,react.useEffect)(()=>{
        var _titleInputRef_current, _titleInputRef_current1;
        (_titleInputRef_current = titleInputRef.current) === null || _titleInputRef_current === void 0 ? void 0 : _titleInputRef_current.focus();
        (_titleInputRef_current1 = titleInputRef.current) === null || _titleInputRef_current1 === void 0 ? void 0 : _titleInputRef_current1.select();
    }, []);
    const handleAddTag = ()=>{
        const trimmed = tagInput.trim();
        if (!trimmed) {
            setTagError("Tag cannot be empty");
            return;
        }
        if (tags.includes(trimmed)) {
            setTagError("Tag already exists");
            return;
        }
        setTags([
            ...tags,
            trimmed
        ]);
        setTagInput("");
        setTagError(null);
    };
    const handleRemoveTag = (tagToRemove)=>{
        setTags(tags.filter((t)=>t !== tagToRemove));
        setTagError(null);
    };
    const handleTagInputKeyDown = (e)=>{
        if (e.key === "Enter") {
            e.preventDefault();
            handleAddTag();
        } else if (e.key === "Escape") {
            e.preventDefault();
            onCancel();
        }
    };
    const handleTitleKeyDown = (e)=>{
        if (e.key === "Enter") {
            e.preventDefault();
            handleSubmit();
        } else if (e.key === "Escape") {
            e.preventDefault();
            onCancel();
        }
    };
    const handleSubmit = async ()=>{
        if (!title.trim() || isSubmitting) return;
        setIsSubmitting(true);
        try {
            await onConfirm({
                title: title.trim(),
                tags
            });
        } catch (e) {
            setIsSubmitting(false);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: overlay,
        onClick: onCancel,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: modal,
            onClick: (e)=>e.stopPropagation(),
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": "resume-modal-title",
            ref: modalRef,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: header,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                            className: ResumeSessionModal_css_title,
                            id: "resume-modal-title",
                            children: "Resume Session"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: subtitle,
                            children: "Edit session details before resuming"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: body,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: fieldGroup,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                    className: fieldLabel,
                                    htmlFor: "resume-title",
                                    children: "Session Title"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                    id: "resume-title",
                                    ref: titleInputRef,
                                    type: "text",
                                    value: title,
                                    onChange: (e)=>setTitle(e.target.value),
                                    onKeyDown: handleTitleKeyDown,
                                    placeholder: "Session title",
                                    className: titleInput
                                }),
                                initialState.hasConflict && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: conflictHint,
                                    children: [
                                        "“",
                                        initialState.originalTitle,
                                        "” is already in use. Suggested: “",
                                        initialState.suggestedTitle,
                                        "”"
                                    ]
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: tagsSection,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                    className: fieldLabel,
                                    children: "Tags"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: tagInputRow,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            type: "text",
                                            value: tagInput,
                                            onChange: (e)=>setTagInput(e.target.value),
                                            onKeyDown: handleTagInputKeyDown,
                                            placeholder: "Add a tag...",
                                            className: ResumeSessionModal_css_tagInput
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            onClick: handleAddTag,
                                            className: addTagButton,
                                            type: "button",
                                            children: "Add"
                                        })
                                    ]
                                }),
                                tagError && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                    className: ResumeSessionModal_css_tagError,
                                    children: tagError
                                }),
                                tags.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                    className: emptyTags,
                                    children: "No tags"
                                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: tagsList,
                                    children: tags.map((tag)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: tagItem,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: tagText,
                                                    children: tag
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    onClick: ()=>handleRemoveTag(tag),
                                                    className: removeTagButton,
                                                    title: 'Remove tag "'.concat(tag, '"'),
                                                    "aria-label": "Remove tag ".concat(tag),
                                                    type: "button",
                                                    children: "\xd7"
                                                })
                                            ]
                                        }, tag))
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: contextSection,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                    className: fieldLabel,
                                    children: "Session Context"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: contextGrid,
                                    children: [
                                        session.branch && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: contextRow,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: contextLabel,
                                                    children: "Branch:"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: contextValue,
                                                    children: session.branch
                                                })
                                            ]
                                        }),
                                        session.program && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: contextRow,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: contextLabel,
                                                    children: "Program:"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: contextValue,
                                                    children: session.program
                                                })
                                            ]
                                        }),
                                        session.existingDir && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: contextRow,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: contextLabel,
                                                    children: "Path:"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: contextValue,
                                                    title: session.existingDir,
                                                    children: session.existingDir
                                                })
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
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            onClick: onCancel,
                            className: cancelButton,
                            disabled: isSubmitting,
                            type: "button",
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            onClick: handleSubmit,
                            className: resumeButton,
                            disabled: !title.trim() || isSubmitting,
                            type: "button",
                            children: isSubmitting ? "Resuming..." : "Resume Session"
                        })
                    ]
                })
            ]
        })
    });
}


/***/ }),

/***/ 93321:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  E: () => (/* binding */ ActionBar)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FActionBar.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8WSy26DMBBF93zFLJOqRiTN05Gq_kk1wIS4gTGynUBa5d8rIKDQvKpuusFifOb6-nr891GZh8ifAXx5ALGyeYoHCeuUypUH9SoKg7mE6luVMFUJC-UosxIiYkemKicVs0czEAIjpzSLEI1IMH-GwJ8ayoYr7-j57Xmj-ryfrITAH1dwjx3fZC_Ql5vo_IKd1OzHzjq1PohIsyN2zdWFdWhcj57eoYnjHju7ztocIxIhuYKIew3z6w1tukfvLaNYIdjIEDEgxzDIsBSFit1GwigYT_JyWIt0mov69_QwXawAx4dy89niptrZQLBuRwJA78msU12IUgLunG6qoqBwq5zodm1kdJoqTiQ4vYs2DdZUq7c6GWDN1Fg9dyBlq9c1nEx1Y3u9EV7h6dy-3RjFWwnB7-KYTYKLOJb_Hsfyr3EsH8XxDcl0doYUBAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FActionBar_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8WSy26DMBBF93zFLJOqRiTN05Gq_kk1wIS4gTGynUBa5d8rIKDQvKpuusFifOb6_nr891GZh8ifAXx5ALGyeYoHCeuUypUH9SoKg7mE6luVMFUJC_UosxIiYkemKicVs0czEAIjpzSLEI1IMH_GwJ8ayoYr7_j57Xmj_ryfrITAH1dwjx3fZC_Ql5vo_IKd1OzHzjq1PohIsyN2zdWFdWhcj57eoYnjHju7ztocIxIhuYKIew3z6w1tukfvLaNYIdjIEDEgxzDIsBSFit1GwigYT_JyWIt0mov69_QwXawAx4dy89niptrZQLBuRwJA78msU12IUgLunG6qoqBwq5zodm1kdJoqTiQ4vYs2DdZUq7c6GWDN1Fg9dyBlq9c1nEx1Y3u9EV7h6dy_3RjFWwnB7_KYTYKLOJb_Hsfyr3EsH8XxDcl0doYUBAAA_22_7D = __webpack_require__(58842);
;// ./src/components/ui/ActionBar.css.ts

var actionBar = '_1xpbanz0';
var ActionBar_css_compact = '_1xpbanz8';
var gapLg = '_1xpbanz3';
var gapMd = '_1xpbanz2';
var gapSm = '_1xpbanz1';
var justifyBetween = '_1xpbanz6';
var justifyCenter = '_1xpbanz7';
var justifyEnd = '_1xpbanz5';
var justifyStart = '_1xpbanz4';
var ActionBar_css_scroll = '_1xpbanz9';
;// ./src/components/ui/ActionBar.tsx
/* __next_internal_client_entry_do_not_use__ ActionBar auto */ 

const gapClass = {
    sm: gapSm,
    md: gapMd,
    lg: gapLg
};
const justifyClass = {
    start: justifyStart,
    end: justifyEnd,
    between: justifyBetween,
    center: justifyCenter
};
function ActionBar(param) {
    let { children, gap = "md", justify = "start", scroll, compact, className, id } = param;
    const classes = [
        actionBar,
        gapClass[gap],
        justifyClass[justify],
        scroll ? ActionBar_css_scroll : undefined,
        compact ? ActionBar_css_compact : undefined,
        className
    ].filter(Boolean).join(" ");
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        id: id,
        className: classes,
        children: children
    });
}


/***/ }),

/***/ 95580:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  y: () => (/* binding */ TagEditor)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/lib/hooks/useFocusTrap.ts
var useFocusTrap = __webpack_require__(63991);
// EXTERNAL MODULE: ./src/lib/sessions/tagProvenance.ts
var sessions_tagProvenance = __webpack_require__(96077);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FTagEditor.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91X247bKhR9n6_gpdJEqiNwLp2Ql_7GeaqIjWNmbOAASZwc9d8rwE7xhYnVdvpwlJEH27Dve63tr2_0WihSUw2-oc3hnKIbBP89AVAoUbsFAEKSjJkrBnD_BMD3JwCMGL5C_tX3p69jgWgo0CjCdSFUjf2yIob-85xC2Sz2jzROn4XDg3d7lp0VqRMghWaGCY5BwRqa751o2SqqaGHapWLHslsfhDGi7m5I9nZU4sRzDM5EPSfJN7Sjt1TdiLMhZ1pW5IpBUdHGPiAVO_KEGVprDDLKDVX28etJG1Zck0xwQ7kJX90SxnPaYIAgdDoJZzXxVv9MElymGlCi6T70cuW8jBu5W3iPVE5VokjOThoDlEpnqiR5zvix9bQmTXJhuSkx2ELod7T3O_ip21FSH6kXeC4n_bf_k5wpmnkPMlGdau6taBJdklxcMIDAph9s7UUdD-QZfgbt33K1iAQB2ZcTQVj7VHfepGvZ-AvatqsgCF12kWyAFhXLBxF7XfRkb5zsmqgj49ZsCF5k4wNWCG4SzW4U33W4R5c2RFufzUxUQg3zAvtatn0tA-molT4pCfUlfRnHwh4VZ6qKSlySKwbkZESXKdc5wfGXwJB7qLyQYNfO7Rql_kgktuHp7SUeDTpdgW3IJh_1shNPy9tUIW_bs9HyR75JoymYDLPDmRY1SFVFOo_gQmQn7bwTJ1MxTjHggtPAzkm99WLcDPa3CloBofQzSNcr2w6DBBOMZUUyWooqp8qpn1Sz7h879OvCxf7eGdH41WH4LiUzNMzVwNtBViYiO9keJ6WtdClYB4jzEnDApa3qBwDIF3eJIxZJkGOgnkySGXamTmiceoITWa9zHTbYX7xdx2W3mgjOxgYnUJOHfekYbNSUdIRUFuejYDIbqpCPIG1MEkTkJCVVmcuJJVJjqEq0ZWPHJ8vNwLhiDHD3Wly1dORUOPoM2TFa3BNeBQqP0xDl2OmiLFDZaxS1yunjEXL_KSJwy9ZCx7Tx-swX-xnod3wH_eZ1SzmrW14Wj9Cr7Jc_8-g-k6uC2o8V-qu3MIhHTnRJIwEJHHFhkERRbnoC30YWpu9YGBaWuVYUA2ZIxbKeyOpOadODTq9K3DZtiDLDSmmHKwTtdBWIr0cWr-bzP-_j_HpWEWaPWPJ36vMR1MV4IHBK_IpTCMW9QukstxD8WL9k3y8YVEXaTuDdvN3d_xF_I8S98WN-ONe2Wu10cx_9UYS2_8Cn0Dwsk7OwrE1ewFk6IxV9RsOJSkZI32-Hy92mv__fwSg18ZHhGPq9L4yJaI3C4pCD8vyOG77mA0vULw11f63bP2LqUx_DY_p_Nx7rDxiP9czx-Ad4ITCkZhIAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTagEditor_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91X247bKhR9n6_gpdJEqiNwLp2Ql_7GeaqIjWNmbOAASZwc9d8rwE7xhYnVdvpwlJEH27Dve63tr2_0WihSUw2_oc3hnKIbBP89AVAoUbsFAEKSjJkrBnD_BMD3JwCMGL5C_tX3p69jgWgo0CjCdSFUjf2yIob_85xC2Sz2jzROn4XDg3d7lp0VqRMghWaGCY5BwRqa751o2SqqaGHapWLHslsfhDGi7m5I9nZU4sRzDM5EPSfJN7Sjt1TdiLMhZ1pW5IpBUdHGPiAVO_KEGVprDDLKDVX28etJG1Zck0xwQ7kJX90SxnPaYIAgdDoJZzXxVv9MElymGlCi6T70cuW8jBu5W3iPVE5VokjOThoDlEpnqiR5zvix9bQmTXJhuSkx2ELod7T3O_ip21FSH6kXeC4n_bf_k5wpmnkPMlGdau6taBJdklxcMIDAph9s7UUdD_QZfgbt33K1iAQB2ZcTQVj7VHfepGvZ_AvatqsgCF12kWyAFhXLBxF7XfRkb5zsmqgj49ZsCF5k4wNWCG4SzW4U33W4R5c2RFufzUxUQg3zAvtatn0tA_molT4pCfUlfRnHwh4VZ6qKSlySKwbkZESXKdc5wfGXwJB7qLyQYNfO7Rql_kgktuHp7SUeDTpdgW3IJh_1shNPy9tUIW_bs9HyR75JoymYDLPDmRY1SFVFOo_gQmQn7bwTJ1MxTjHggtPAzkm99WLcDPa3CloBofQzSNcr2w6DBBOMZUUyWooqp8qpn1Sz7h879OvCxf7eGdH41WH4LiUzNMzVwNtBViYiO9keJ6WtdClYB4jzEnDApa3qBwDIF3eJIxZJkGOgnkySGXamTmiceoITWa9zHTbYX7xdx2W3mgjOxgYnUJOHfekYbNSUdIRUFuejYDIbqpCPIG1MEkTkJCVVmcuJJVJjqEq0ZWPHJ8vNwLhiDHD3Wly1dORUOPoM2TFa3BNeBQqP0xDl2OmiLFDZaxS1yunjEXL_KSJwy9ZCx7Tx_swX_xnod3wH_eZ1SzmrW14Wj9Cr7Jc_8_g_k6uC2o8V_qu3MIhHTnRJIwEJHHFhkERRbnoC30YWpu9YGBaWuVYUA2ZIxbKeyOpOadODTq9K3DZtiDLDSmmHKwTtdBWIr0cWr_bzP_j_HpWEWaPWPJ36vMR1MV4IHBK_IpTCMW9QukstxD8WL9k3y8YVEXaTuDdvN3d_xF_I8S98WN_ONe2Wu10cx_9UYS2_8Cn0Dwsk7OwrE1ewFk6IxV9RsOJSkZI32_Hy92mv_fwSg18ZHhGPq9L4yJaI3C4pCD8vyOG77mA0vULw11f63bP2LqUx_DY_p_Nx7rDxiP9czx_Ad4ITCkZhIAAA_22_7D = __webpack_require__(29087);
;// ./src/components/sessions/TagEditor.css.ts


var addButton = '_15bv21zb';
var body = '_15bv21z7';
var cancelButton = '_15bv21zr';
var confirmCaption = '_15bv21zm';
var emptyMessage = '_15bv21zf';
var TagEditor_css_error = '_15bv21zc';
var footer = '_15bv21zq';
var header = '_15bv21z4';
var input = '_15bv21za';
var inputRow = '_15bv21z9';
var inputSection = '_15bv21z8';
var keepButton = '_15bv21zn';
var modal = '_15bv21z3';
var overlay = '_15bv21z2';
var removeAnywayButton = '_15bv21zo';
var removeButton = '_15bv21zp';
var saveButton = '_15bv21zs';
var sectionTitle = '_15bv21ze';
var subtitle = '_15bv21z6';
var tagItem = '_15bv21zh';
var tagItemConfirming = '_15bv21zl';
var tagItemUnclassified = '_15bv21zj';
var tagText = '_15bv21zi';
var tagsList = '_15bv21zg';
var tagsSection = '_15bv21zd';
var title = '_15bv21z5';
var unclassifiedCaption = '_15bv21zk';
;// ./src/components/sessions/TagEditor.tsx
/* __next_internal_client_entry_do_not_use__ TagEditor auto */ 








function TagEditor(param) {
    let { tags, tagProvenance, tagRuleNames = {}, onSave, onCancel, sessionTitle, triggerRef, sessionId, onReclassified } = param;
    const [currentTags, setCurrentTags] = (0,react.useState)([
        ...tags
    ]);
    const modalRef = (0,react.useRef)(null);
    (0,useFocusTrap/* useFocusTrap */.r)(modalRef, true, triggerRef);
    const [inputValue, setInputValue] = (0,react.useState)("");
    const [error, setError] = (0,react.useState)(null);
    const [reclassifying, setReclassifying] = (0,react.useState)(false);
    const [reclassifyStatus, setReclassifyStatus] = (0,react.useState)(null);
    // ux.md Surface 3: at most one tag's removal confirmation is expanded at a time.
    const [pendingRemoval, setPendingRemoval] = (0,react.useState)(null);
    const keepButtonRef = (0,react.useRef)(null);
    const handleAddTag = ()=>{
        const trimmedTag = inputValue.trim();
        if (!trimmedTag) {
            setError("Tag cannot be empty");
            return;
        }
        if (currentTags.includes(trimmedTag)) {
            setError("Tag already exists");
            return;
        }
        setCurrentTags([
            ...currentTags,
            trimmedTag
        ]);
        setInputValue("");
        setError(null);
    };
    const removeImmediately = (tagToRemove)=>{
        setCurrentTags((prev)=>prev.filter((tag)=>tag !== tagToRemove));
        setError(null);
        setPendingRemoval(null);
    };
    const handleRemoveClick = (tagToRemove)=>{
        if ((0,sessions_tagProvenance/* hasProvenance */.MV)(tagToRemove, tagProvenance)) {
            setPendingRemoval(tagToRemove);
            // Focus the non-destructive default (WCAG 2.4.3, ux.md AC13).
            setTimeout(()=>{
                var _keepButtonRef_current;
                return (_keepButtonRef_current = keepButtonRef.current) === null || _keepButtonRef_current === void 0 ? void 0 : _keepButtonRef_current.focus();
            }, 0);
            return;
        }
        removeImmediately(tagToRemove);
    };
    const handleKeepRemoval = ()=>setPendingRemoval(null);
    const handleKeyPress = (e)=>{
        if (e.key === "Enter") {
            e.preventDefault();
            handleAddTag();
        } else if (e.key === "Escape") {
            onCancel();
        }
    };
    const handleSave = ()=>{
        onSave(currentTags);
    };
    // Manual AI re-classification: successfully classified sessions are never re-tagged
    // automatically (poller classify-once), so this explicit user action is the only path
    // that re-runs the LLM classifier. The server applies the result; the returned tags
    // refresh this dialog and propagate to the parent via onReclassified.
    const handleReclassify = async ()=>{
        if (!sessionId || reclassifying) return;
        setReclassifying(true);
        setReclassifyStatus(null);
        setError(null);
        try {
            const transport = (0,connect_transport/* createConnectTransport */.w)({
                baseUrl: (0,config/* getApiBaseUrl */.h)()
            });
            const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
            const response = await client.reclassifySessionTags({
                sessionId
            });
            const applied = [
                ...response.tags
            ];
            setCurrentTags(applied);
            setReclassifyStatus(applied.length > 0 ? "AI classification applied: ".concat(applied.join(", ")) : "AI classification ran with no tags");
            onReclassified === null || onReclassified === void 0 ? void 0 : onReclassified(applied);
        } catch (e) {
            setError(e instanceof Error ? "Re-classification failed: ".concat(e.message) : "Re-classification failed");
        } finally{
            setReclassifying(false);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: overlay,
        onClick: onCancel,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: modal,
            onClick: (e)=>e.stopPropagation(),
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": "tag-editor-title",
            ref: modalRef,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: header,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                            className: title,
                            id: "tag-editor-title",
                            children: "Edit Tags"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: subtitle,
                            children: sessionTitle
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: body,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: inputSection,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: inputRow,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            type: "text",
                                            value: inputValue,
                                            onChange: (e)=>setInputValue(e.target.value),
                                            onKeyDown: handleKeyPress,
                                            placeholder: "Add a new tag...",
                                            className: input,
                                            autoFocus: true
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            type: "button",
                                            onClick: handleAddTag,
                                            className: addButton,
                                            children: "Add"
                                        })
                                    ]
                                }),
                                error && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                    className: TagEditor_css_error,
                                    children: error
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: tagsSection,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("h3", {
                                    className: sectionTitle,
                                    children: [
                                        "Current Tags (",
                                        currentTags.length,
                                        ")"
                                    ]
                                }),
                                currentTags.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                    className: emptyMessage,
                                    children: "No tags yet. Add your first tag above."
                                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: tagsList,
                                    children: currentTags.map((tag)=>{
                                        const isUnclassified = tag === sessions_tagProvenance/* UNCLASSIFIED_TAG */.Uw;
                                        const isPendingRemoval = pendingRemoval === tag;
                                        if (isPendingRemoval) {
                                            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: "".concat(tagItem, " ").concat(tagItemConfirming),
                                                "data-testid": "tag-confirm-row-".concat(tag),
                                                onKeyDown: (e)=>{
                                                    if (e.key === "Escape") {
                                                        e.stopPropagation();
                                                        handleKeepRemoval();
                                                    }
                                                },
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: tagText,
                                                        children: tag
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: confirmCaption,
                                                        children: (0,sessions_tagProvenance/* removalConfirmationText */.JG)(tag, tagProvenance, tagRuleNames)
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        ref: keepButtonRef,
                                                        type: "button",
                                                        onClick: handleKeepRemoval,
                                                        className: keepButton,
                                                        children: "Keep"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        type: "button",
                                                        onClick: ()=>removeImmediately(tag),
                                                        className: removeAnywayButton,
                                                        children: "Remove anyway"
                                                    })
                                                ]
                                            }, tag);
                                        }
                                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: "".concat(tagItem, " ").concat(isUnclassified ? tagItemUnclassified : ""),
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: tagText,
                                                    children: tag
                                                }),
                                                isUnclassified ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: unclassifiedCaption,
                                                    children: "Removed automatically once classification succeeds"
                                                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    onClick: ()=>handleRemoveClick(tag),
                                                    className: removeButton,
                                                    title: 'Remove tag "'.concat(tag, '"'),
                                                    "aria-label": "Remove tag ".concat(tag),
                                                    children: "\xd7"
                                                })
                                            ]
                                        }, tag);
                                    })
                                })
                            ]
                        })
                    ]
                }),
                reclassifyStatus && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: unclassifiedCaption,
                    role: "status",
                    children: reclassifyStatus
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: footer,
                    children: [
                        sessionId && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            onClick: handleReclassify,
                            className: cancelButton,
                            disabled: reclassifying,
                            "data-testid": "tag-reclassify-button",
                            title: "Re-run AI tag classification for this session",
                            children: reclassifying ? "Classifying…" : "Re-run AI classification"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            onClick: onCancel,
                            className: cancelButton,
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            onClick: handleSave,
                            className: saveButton,
                            children: "Save Tags"
                        })
                    ]
                })
            ]
        })
    });
}


/***/ }),

/***/ 96077:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   JG: () => (/* binding */ removalConfirmationText),
/* harmony export */   MV: () => (/* binding */ hasProvenance),
/* harmony export */   Uw: () => (/* binding */ UNCLASSIFIED_TAG),
/* harmony export */   bT: () => (/* binding */ tagProvenanceAriaLabel),
/* harmony export */   en: () => (/* binding */ tagProvenanceTitle)
/* harmony export */ });
/* unused harmony exports LLM_PROVENANCE_SENTINEL, UNCLASSIFIED_TOOLTIP, resolveProvenanceRuleName, isLlmProvenance */
/**
 * Shared tag-provenance helpers (ux.md Surfaces 1–3, Epic 6.2). A tag's entry
 * in `Session.ruleTagProvenance` is either a `TaggingRule.ID`, the sentinel
 * "llm" (LLM-classifier-derived), or absent (a plain user-added tag).
 */ /** Sentinel value the LLM-fallback poller uses when classification fails/times out. */ const UNCLASSIFIED_TAG = "Unclassified";
/** Sentinel provenance value for LLM-classifier-derived tags (session.Instance's "llm" key). */ const LLM_PROVENANCE_SENTINEL = "llm";
/** Tooltip text for the transient Unclassified sentinel pill (ux.md AC8). */ const UNCLASSIFIED_TOOLTIP = "LLM classification failed or timed out — will retry next poll cycle";
/**
 * Returns the rule-provenance display name for `tag`, or null if it has no
 * provenance entry (a plain user tag). Degrades to a generic name when the
 * provenance rule ID doesn't resolve (deleted rule) — never returns
 * undefined/blank (ux.md's explicit edge case for both Surface 1 and 3).
 */ function resolveProvenanceRuleName(tag, provenance, ruleNames) {
    const entry = provenance === null || provenance === void 0 ? void 0 : provenance[tag];
    if (!entry) return null;
    if (entry === LLM_PROVENANCE_SENTINEL) return null; // handled separately — not a rule name
    var _ruleNames_entry;
    return (_ruleNames_entry = ruleNames[entry]) !== null && _ruleNames_entry !== void 0 ? _ruleNames_entry : null;
}
/** True if `tag` has any provenance entry (rule- or LLM-derived) — i.e. not a plain user tag. */ function hasProvenance(tag, provenance) {
    return !!(provenance === null || provenance === void 0 ? void 0 : provenance[tag]);
}
/** True if `tag`'s provenance entry is the LLM sentinel. */ function isLlmProvenance(tag, provenance) {
    return (provenance === null || provenance === void 0 ? void 0 : provenance[tag]) === LLM_PROVENANCE_SENTINEL;
}
/** `title` tooltip text for a tag pill (ux.md Surface 1). Undefined for plain user tags. */ function tagProvenanceTitle(tag, provenance, ruleNames) {
    if (tag === UNCLASSIFIED_TAG) return UNCLASSIFIED_TOOLTIP;
    const entry = provenance === null || provenance === void 0 ? void 0 : provenance[tag];
    if (!entry) return undefined;
    if (entry === LLM_PROVENANCE_SENTINEL) return "Applied by AI classification";
    const ruleName = ruleNames[entry];
    return ruleName ? "Applied by rule: ".concat(ruleName) : "Applied automatically — originating rule no longer exists";
}
/** Accessible name for a tag pill (ux.md AC2/AC4). Manual tags get the plain `Tag: {name}` form. */ function tagProvenanceAriaLabel(tag, provenance) {
    if (tag === UNCLASSIFIED_TAG) return "Tag: ".concat(tag, " (auto-applied, will retry)");
    const entry = provenance === null || provenance === void 0 ? void 0 : provenance[tag];
    if (!entry) return "Tag: ".concat(tag);
    return entry === LLM_PROVENANCE_SENTINEL ? "Tag: ".concat(tag, " (auto-applied by AI)") : "Tag: ".concat(tag, " (auto-applied by rule)");
}
/**
 * The "may reappear" removal-confirmation text for a rule-provenance tag
 * (ux.md Surface 3, AC11) — falls back to generic wording when the rule name
 * can't be resolved, never blank/undefined.
 */ function removalConfirmationText(tag, provenance, ruleNames) {
    const entry = provenance === null || provenance === void 0 ? void 0 : provenance[tag];
    if (entry && entry !== LLM_PROVENANCE_SENTINEL) {
        const ruleName = ruleNames[entry];
        if (ruleName) return 'Applied by rule "'.concat(ruleName, '" — may reappear.');
    }
    return "This tag was applied automatically and may reappear — remove anyway?";
}


/***/ }),

/***/ 97443:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  J: () => (/* binding */ BacklogOriginBadge)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fshared%2FBacklogOriginBadge.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll81c3NhbmEwIHsKICBkaXNwbGF5OiBpbmxpbmUtZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogNHB4OwogIHBhZGRpbmc6IDRweCAxMHB4OwogIGJvcmRlci1yYWRpdXM6IDEycHg7CiAgZm9udC1zaXplOiAxMnB4OwogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgdGV4dC1kZWNvcmF0aW9uOiBub25lOwogIGN1cnNvcjogcG9pbnRlcjsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Yyk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7Cn0KLl81c3NhbmEwOmhvdmVyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwogIGJvcmRlci1jb2xvcjogdmFyKC0tXzE5ZXoycnppKTsKfQouXzVzc2FuYTA6Zm9jdXMgewogIG91dGxpbmU6IDJweCBzb2xpZCB2YXIoLS1fMTllejJyem0pOwogIG91dGxpbmUtb2Zmc2V0OiAycHg7Cn0KLl81c3NhbmExIHsKICBwYWRkaW5nOiA0cHggOHB4OwogIGZvbnQtc2l6ZTogMTFweDsKfQouXzVzc2FuYTIgewogIHdpZHRoOiAxNHB4OwogIGhlaWdodDogMTRweDsKICBmbGV4LXNocmluazogMDsKfQouXzVzc2FuYTMgewogIHdoaXRlLXNwYWNlOiBub3dyYXA7CiAgb3ZlcmZsb3c6IGhpZGRlbjsKICB0ZXh0LW92ZXJmbG93OiBlbGxpcHNpczsKICBtYXgtd2lkdGg6IDEyMHB4OwogIHRleHQtdHJhbnNmb3JtOiBjYXBpdGFsaXplOwp9CkBtZWRpYSBzY3JlZW4gYW5kIChtYXgtd2lkdGg6IDc2OHB4KSB7CiAgLl81c3NhbmEwIHsKICAgIGZvbnQtc2l6ZTogMTFweDsKICAgIHBhZGRpbmc6IDNweCA4cHg7CiAgfQogIC5fNXNzYW5hMiB7CiAgICB3aWR0aDogMTJweDsKICAgIGhlaWdodDogMTJweDsKICB9Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FBacklogOriginBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll81c3NhbmEwIHsKICBkaXNwbGF5OiBpbmxpbmUtZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogNHB4OwogIHBhZGRpbmc6IDRweCAxMHB4OwogIGJvcmRlci1yYWRpdXM6IDEycHg7CiAgZm9udC1zaXplOiAxMnB4OwogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgdGV4dC1kZWNvcmF0aW9uOiBub25lOwogIGN1cnNvcjogcG9pbnRlcjsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Yyk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7Cn0KLl81c3NhbmEwOmhvdmVyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwogIGJvcmRlci1jb2xvcjogdmFyKC0tXzE5ZXoycnppKTsKfQouXzVzc2FuYTA6Zm9jdXMgewogIG91dGxpbmU6IDJweCBzb2xpZCB2YXIoLS1fMTllejJyem0pOwogIG91dGxpbmUtb2Zmc2V0OiAycHg7Cn0KLl81c3NhbmExIHsKICBwYWRkaW5nOiA0cHggOHB4OwogIGZvbnQtc2l6ZTogMTFweDsKfQouXzVzc2FuYTIgewogIHdpZHRoOiAxNHB4OwogIGhlaWdodDogMTRweDsKICBmbGV4LXNocmluazogMDsKfQouXzVzc2FuYTMgewogIHdoaXRlLXNwYWNlOiBub3dyYXA7CiAgb3ZlcmZsb3c6IGhpZGRlbjsKICB0ZXh0LW92ZXJmbG93OiBlbGxpcHNpczsKICBtYXgtd2lkdGg6IDEyMHB4OwogIHRleHQtdHJhbnNmb3JtOiBjYXBpdGFsaXplOwp9CkBtZWRpYSBzY3JlZW4gYW5kIChtYXgtd2lkdGg6IDc2OHB4KSB7CiAgLl81c3NhbmEwIHsKICAgIGZvbnQtc2l6ZTogMTFweDsKICAgIHBhZGRpbmc6IDNweCA4cHg7CiAgfQogIC5fNXNzYW5hMiB7CiAgICB3aWR0aDogMTJweDsKICAgIGhlaWdodDogMTJweDsKICB9Cn0_22_7D = __webpack_require__(20832);
;// ./src/components/shared/BacklogOriginBadge.css.ts

var badge = '_5ssana0';
var BacklogOriginBadge_css_compact = '_5ssana1';
var icon = '_5ssana2';
var BacklogOriginBadge_css_text = '_5ssana3';
;// ./src/components/shared/BacklogOriginBadge.tsx
/* __next_internal_client_entry_do_not_use__ BacklogOriginBadge auto */ 
// +feature: backlog-origin-badge


const KNOWN_ROLES = new Set([
    "work",
    "review",
    "triage",
    "diagnose"
]);
/**
 * Badge linking a session back to the backlog item that spawned it (work/review/triage
 * automation). Renders nothing when the session has no backlog link.
 */ function BacklogOriginBadge(param) {
    let { entry, compact = false } = param;
    if (!(entry === null || entry === void 0 ? void 0 : entry.itemId)) return null;
    // sessionRole is a free-form string set server-side (session role, not a proto enum) —
    // fall back to a generic label for anything unexpected rather than showing "undefined".
    const roleLabel = entry.sessionRole && KNOWN_ROLES.has(entry.sessionRole) ? entry.sessionRole : "backlog";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(nav/* Link */.N_, {
        href: "/backlog?item=".concat(encodeURIComponent(entry.itemId)),
        className: "".concat(badge, " ").concat(compact ? BacklogOriginBadge_css_compact : ""),
        onClick: (e)=>e.stopPropagation(),
        title: "Backlog item: ".concat(entry.itemTitle || entry.itemId).concat(entry.sessionRole ? " (".concat(entry.sessionRole, ")") : ""),
        "aria-label": "View originating backlog item, role: ".concat(roleLabel),
        "data-testid": "backlog-origin-badge",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("svg", {
                className: icon,
                viewBox: "0 0 16 16",
                fill: "currentColor",
                "aria-hidden": "true",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("path", {
                    d: "M2 2.75A.75.75 0 0 1 2.75 2h10.5a.75.75 0 0 1 .75.75v10.5a.75.75 0 0 1-.75.75H2.75a.75.75 0 0 1-.75-.75V2.75Zm1.5.75v9h9v-9h-9Zm1.5 1.5h6v1.5h-6V5Zm0 3h6v1.5h-6V8Zm0 3h4v1.5h-4V11Z"
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: BacklogOriginBadge_css_text,
                children: roleLabel
            })
        ]
    });
}


/***/ }),

/***/ 99050:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Uo: () => (/* binding */ mobileDebug)
/* harmony export */ });
/* unused harmony exports DEBUG_FLAG_KEY, MAX_DEBUG_ENTRIES, MISROUTE_OVERRIDE_WINDOW_MS, MISROUTE_TOOLBAR_WINDOW_MS, MobileDebugLog */
/**
 * Local-only mobile terminal debug log. Off unless
 * localStorage['debug-terminal-mobile'] === 'true'; when off every method is a
 * no-op (no buffering, no console output, no stats). Nothing is sent anywhere.
 * Read it from a phone via window.__termDebug.dump() / .stats().
 */ const DEBUG_FLAG_KEY = "debug-terminal-mobile";
const MAX_DEBUG_ENTRIES = 500;
const MISROUTE_OVERRIDE_WINDOW_MS = 10000;
const MISROUTE_TOOLBAR_WINDOW_MS = 5000;
const AUTO_SOURCE = "auto";
function readLocalStorageFlag() {
    try {
        return typeof localStorage !== "undefined" && localStorage.getItem(DEBUG_FLAG_KEY) === "true";
    } catch (e) {
        return false; // storage can throw in private mode / sandboxed iframes
    }
}
class MobileDebugLog {
    /** Registers window.__termDebug. Safe to call repeatedly. */ install() {
        if (!this.target) return;
        this.target.__termDebug = {
            dump: ()=>this.dump(),
            stats: ()=>this.stats()
        };
    }
    /** Current flag value, for callers that cache it to skip building log payloads while off. */ enabled() {
        return this.isEnabled();
    }
    log(type, data) {
        if (!this.isEnabled()) return;
        const entry = {
            t: this.now(),
            type,
            data
        };
        this.ring[this.head] = entry;
        this.head = (this.head + 1) % MAX_DEBUG_ENTRIES;
        if (this.size < MAX_DEBUG_ENTRIES) this.size++;
        this.sink(entry);
    }
    /** JSON array of buffered entries, oldest first. */ dump() {
        const start = this.size < MAX_DEBUG_ENTRIES ? 0 : this.head;
        const entries = [];
        for(let i = 0; i < this.size; i++){
            entries.push(this.ring[(start + i) % MAX_DEBUG_ENTRIES]);
        }
        return JSON.stringify(entries);
    }
    /** Counts since page load, or null when the debug flag is off. */ stats() {
        if (!this.isEnabled()) return null;
        return {
            routeDecisions: this.routeDecisions,
            overrideChanges: this.overrideChanges,
            misroutes: {
                overrideAfterDrag: this.overrideAfterDrag,
                toolbarKeyAfterDrag: this.toolbarKeyAfterDrag
            }
        };
    }
    routeDecision(decision) {
        if (!this.isEnabled()) return;
        this.routeDecisions++;
        this.lastAutoDragAt = decision.source === AUTO_SOURCE ? this.now() : null;
        this.log("route-decision", decision);
    }
    overrideChange(from, to) {
        if (!this.isEnabled()) return;
        this.overrideChanges++;
        this.log("override-change", {
            from,
            to
        });
        if (this.withinWindow(MISROUTE_OVERRIDE_WINDOW_MS)) {
            this.overrideAfterDrag++;
            this.log("misroute-proxy", {
                kind: "override-after-drag"
            });
        }
    }
    toolbarKey(key) {
        if (!this.isEnabled()) return;
        if (this.withinWindow(MISROUTE_TOOLBAR_WINDOW_MS)) {
            this.toolbarKeyAfterDrag++;
            this.log("misroute-proxy", {
                kind: "toolbar-key-after-drag"
            });
        }
    }
    withinWindow(windowMs) {
        return this.lastAutoDragAt !== null && this.now() - this.lastAutoDragAt <= windowMs;
    }
    constructor(options = {}){
        this.ring = new Array(MAX_DEBUG_ENTRIES);
        this.head = 0 // next write slot
        ;
        this.size = 0;
        this.lastAutoDragAt = null;
        this.routeDecisions = 0;
        this.overrideChanges = 0;
        this.overrideAfterDrag = 0;
        this.toolbarKeyAfterDrag = 0;
        var _options_now;
        this.now = (_options_now = options.now) !== null && _options_now !== void 0 ? _options_now : Date.now;
        var _options_isEnabled;
        this.isEnabled = (_options_isEnabled = options.isEnabled) !== null && _options_isEnabled !== void 0 ? _options_isEnabled : readLocalStorageFlag;
        var _options_sink;
        this.sink = (_options_sink = options.sink) !== null && _options_sink !== void 0 ? _options_sink : (entry)=>console.debug("[term-debug]", entry.type, entry.data);
        var _options_target;
        this.target = (_options_target = options.target) !== null && _options_target !== void 0 ? _options_target :  true ? window : 0;
    }
}
/** Shared instance for app code; registered on window at first import in the browser. */ const mobileDebug = new MobileDebugLog();
mobileDebug.install();


/***/ })

}]);
//# sourceMappingURL=3889-70f712ae7ca2ee78.js.map