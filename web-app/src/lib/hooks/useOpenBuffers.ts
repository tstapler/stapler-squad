"use client";

import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from "react";
import {
  DESKTOP_CAP, MOBILE_CAP, bufferReducer, initialBufferState, mruPaths, selectedPath as selectedPathOf, activeBuffer,
  type BufferAction, type BufferMode, type BufferState, type FailureKind, type ReadOnlyKind, type ViewState,
} from "@/lib/files/bufferReducer";
import { bucketBytes, bucketTabs, extClass, type LabelsOf } from "@/lib/files/analytics/events";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";
import { Rejected } from "@/lib/files/transport/fileTransport";
import { useFileTransport } from "./useFileTransport";

const MOBILE_WIDTH = 768;
const PERSIST_DEBOUNCE_MS = 300;
const STORAGE_VERSION = 1;

function classify(err: unknown): FailureKind {
  if (!(err instanceof Rejected)) return "rpc_error";
  return err.reason === "not_found" || err.reason === "too_large" || err.reason === "binary" ? err.reason : "rpc_error";
}

const formFactor = () => (typeof window !== "undefined" && window.innerWidth < MOBILE_WIDTH ? "mobile" : "desktop");
const capFor = (form: string, coarse: boolean) => (form === "mobile" || coarse ? MOBILE_CAP : DESKTOP_CAP);
const coarsePointer = () => typeof window !== "undefined" && typeof window.matchMedia === "function" && window.matchMedia("(pointer: coarse)").matches;
export const storageKey = (sessionId: string, device: string) => `ssq.files.${sessionId}.${device}`;

interface Persisted { v: number; active: number; tabs: { path: string; line: number; col: number; scrollTop: number }[] }
type RestoreOutcome = "ok" | "partial" | "storage_unavailable";
interface Restored { tabs: { path: string; viewState: ViewState }[]; active: number; outcome: RestoreOutcome | null }

const isNum = (n: unknown): n is number => typeof n === "number" && Number.isFinite(n);

function readPersisted(sessionId: string): Restored {
  try {
    const raw = window.localStorage.getItem(storageKey(sessionId, formFactor()));
    if (!raw) return { tabs: [], active: 0, outcome: null };
    const data = JSON.parse(raw) as Partial<Persisted>;
    const rows = Array.isArray(data.tabs) ? data.tabs : [];
    const tabs = rows
      .filter((t) => t && typeof t.path === "string" && t.path && isNum(t.line) && isNum(t.col) && isNum(t.scrollTop))
      .map((t) => ({ path: t.path, viewState: { line: t.line, col: t.col, scrollTop: t.scrollTop } }));
    const ok = data.v === STORAGE_VERSION && tabs.length === rows.length;
    return { tabs, active: isNum(data.active) ? data.active : 0, outcome: ok ? "ok" : "partial" };
  } catch {
    return { tabs: [], active: 0, outcome: "storage_unavailable" };
  }
}

function writePersisted(sessionId: string, state: BufferState): boolean {
  try {
    const payload: Persisted = {
      v: STORAGE_VERSION,
      active: state.activeIndex,
      tabs: state.buffers.map((b) => ({ path: b.path, line: b.viewState.line, col: b.viewState.col, scrollTop: b.viewState.scrollTop })),
    };
    window.localStorage.setItem(storageKey(sessionId, formFactor()), JSON.stringify(payload));
    return true;
  } catch {
    return false;
  }
}

export type OpenSource = LabelsOf<"files.tab.open">["source"];

export interface UseOpenBuffersOptions {
  initialSelectedPath?: string | null;
  onSelectedPathChange?: (path: string | null) => void;
}

export function useOpenBuffers(sessionId: string, { initialSelectedPath, onSelectedPathChange }: UseOpenBuffersOptions = {}) {
  const restoredRef = useRef<Restored | null>(null);
  const [state, dispatchRaw] = useReducer(bufferReducer, undefined, () => {
    const base = initialBufferState(capFor(formFactor(), coarsePointer()));
    const restored = typeof window === "undefined" ? { tabs: [], active: 0, outcome: null } : readPersisted(sessionId);
    restoredRef.current = restored;
    return restored.tabs.length ? bufferReducer(base, { type: "restore", sessionId, tabs: restored.tabs, activeIndex: restored.active }) : base;
  });
  const [storageFailed, setStorageFailed] = useState(restoredRef.current?.outcome === "storage_unavailable");
  const analytics = useFilesAnalytics({ component: "FilesTab", sessionId });
  const transport = useFileTransport();
  const dispatch = useCallback((a: BufferAction) => dispatchRaw(a), []);

  // One restore event per mount, after the buffers exist.
  useEffect(() => {
    const restored = restoredRef.current;
    if (restored?.outcome) analytics.emit("files.tab.restore", { outcome: restored.outcome, tabs: bucketTabs(restored.tabs.length) });
    restoredRef.current = null;
  }, [analytics]);

  // Viewport and pointer decide the tab cap; both are re-read on resize.
  useEffect(() => {
    const update = () => dispatchRaw({ type: "setCap", cap: capFor(formFactor(), coarsePointer()) });
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, []);

  // Where each newly opened path came from; consumed by `files.tab.open` once its stat lands.
  const sources = useRef(new Map<string, { source: OpenSource; pinned: boolean }>());
  const stateRef = useRef(state);
  stateRef.current = state;
  const noteSource = useCallback((path: string, source: OpenSource, pinned: boolean) => {
    if (!stateRef.current.buffers.some((b) => b.path === path)) sources.current.set(path, { source, pinned });
  }, []);

  // Load each new buffer once; a stat is enough, the viewer owns the text.
  const inflight = useRef(new Map<string, AbortController>());
  useEffect(() => {
    for (const b of state.buffers) {
      if (b.status !== "loading") continue;
      const key = `${b.sessionId}\0${b.path}`;
      if (inflight.current.has(key)) continue;
      const ctl = new AbortController();
      inflight.current.set(key, ctl);
      transport.stat({ sessionId: b.sessionId, path: b.path }, undefined, ctl.signal).then(
        (r) => {
          if (ctl.signal.aborted) return;
          dispatchRaw({ type: "markLoaded", path: b.path, token: r.token, readOnly: r.readOnly });
          const origin = sources.current.get(b.path);
          sources.current.delete(b.path);
          if (origin) analytics.emit("files.tab.open", { kind: origin.pinned ? "pinned" : "preview", source: origin.source, ext_class: extClass(b.path), size: bucketBytes(r.size) });
        },
        (err) => { if (!ctl.signal.aborted) dispatchRaw({ type: "markFailed", path: b.path, reason: classify(err) }); },
      ).finally(() => inflight.current.delete(key));
    }
  }, [state.buffers, transport, analytics]);
  useEffect(() => {
    const map = inflight.current;
    return () => {
      map.forEach((c) => c.abort());
      map.clear();
    };
  }, []);

  // Persist paths, active index and positions; text is never stored.
  const flush = useCallback(() => {
    if (!writePersisted(sessionId, stateRef.current)) setStorageFailed(true);
  }, [sessionId]);
  useEffect(() => {
    const t = setTimeout(flush, PERSIST_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [state.buffers, state.activeIndex, flush]);
  useEffect(() => {
    window.addEventListener("pagehide", flush);
    return () => {
      window.removeEventListener("pagehide", flush);
      flush();
    };
  }, [flush]);

  // Back-compat with the single-path props (VCS cross-link).
  const selected = selectedPathOf(state);
  useEffect(() => {
    if (initialSelectedPath && initialSelectedPath !== selected) {
      noteSource(initialSelectedPath, "deeplink", false);
      dispatchRaw({ type: "open", path: initialSelectedPath, sessionId });
    }
    // Only a changed prop opens a file; selection changes must not re-open the old prop.
  }, [initialSelectedPath]); // eslint-disable-line react-hooks/exhaustive-deps
  const notified = useRef<string | null | undefined>(undefined);
  useEffect(() => {
    if (notified.current === selected || (notified.current === undefined && selected === null)) return;
    notified.current = selected;
    onSelectedPathChange?.(selected);
  }, [selected, onSelectedPathChange]);

  const actions = useMemo(() => ({
    open: (path: string, opts: { pinned?: boolean; source?: OpenSource } = {}) => {
      noteSource(path, opts.source ?? "tree", !!opts.pinned);
      dispatchRaw({ type: "open", path, sessionId, pinned: opts.pinned });
    },
    jump: (path: string, line: number, opts: { col?: number; pinned?: boolean; source?: OpenSource; chain?: "start" | "step" } = {}) => {
      noteSource(path, opts.source ?? "jumplist", !!opts.pinned);
      dispatchRaw({ type: "jump", path, sessionId, line, col: opts.col ?? 0, pinned: opts.pinned, chain: opts.chain });
    },
    move: (path: string, delta: -1 | 1) => dispatchRaw({ type: "move", path, delta }),
    adoptToken: (path: string, token: string) => dispatchRaw({ type: "adoptToken", path, token }),
    revealed: (seq: number) => dispatchRaw({ type: "revealed", seq }),
    close: (path: string, discardConfirmed = false) => dispatchRaw({ type: "close", path, discardConfirmed }),
    activate: (path: string) => dispatchRaw({ type: "activate", path }),
    pin: (path: string) => dispatchRaw({ type: "pin", path }),
    unpin: (path: string) => dispatchRaw({ type: "unpin", path }),
    navBack: () => dispatchRaw({ type: "navBack" }),
    navForward: () => dispatchRaw({ type: "navForward" }),
    recordViewState: (path: string, viewState: ViewState) => dispatchRaw({ type: "recordViewState", path, viewState }),
    setMode: (path: string, mode: BufferMode) => dispatchRaw({ type: "setMode", path, mode }),
    markDirty: (path: string, dirty: boolean) => dispatchRaw({ type: "markDirty", path, dirty }),
    reopenClosed: () => dispatchRaw({ type: "reopenClosed" }),
    clearNotice: () => dispatchRaw({ type: "clearNotice" }),
  }), [sessionId, noteSource]);

  return {
    state,
    dispatch,
    active: activeBuffer(state),
    selectedPath: selected,
    mru: useMemo(() => mruPaths(state), [state]),
    /** True once localStorage failed: show the one-time "Tab layout will not be remembered" status. */
    storageUnavailable: storageFailed,
    ...actions,
  };
}
