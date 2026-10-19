"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  bucketFiles, bucketQueryLen, bucketResults, bucketScale, searchOpts, type LabelsOf,
} from "@/lib/files/analytics/events";
import { useFilesFlags } from "@/lib/hooks/useFilesFlags";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";
import {
  MIN_QUERY_LENGTH, classifySearchError, connectSearchStream, newClientId,
  type SearchEvent, type SearchHit, type SearchQuery, type SearchStream,
} from "@/lib/files/searchClient";

export const SEARCH_DEBOUNCE_MS = 300;
export const HIDDEN_ABORT_MS = 30_000;

export type SearchStatus = "idle" | "queued" | "searching" | "done" | "cancelled" | "busy" | "invalid" | "error" | "worktree_unavailable" | "disabled";
type Stats = Extract<SearchEvent, { type: "stats" }>;
type Outcome = LabelsOf<"files.search.results">["outcome"];

export interface SearchState {
  status: SearchStatus;
  /** Searches ahead of this one while `queued`. */
  ahead: number;
  hits: SearchHit[];
  /** Hits belong to an earlier query: shown dimmed until the first new batch arrives. */
  stale: boolean;
  stats: Stats | null;
  /** Server-provided text for `invalid`. */
  message: string | null;
  /** The query `hits` answer, for the empty-state wording. */
  answered: SearchQuery | null;
}

const INITIAL: SearchState = { status: "idle", ahead: 0, hits: [], stale: false, stats: null, message: null, answered: null };
const now = () => (typeof performance !== "undefined" ? performance.now() : Date.now());
const distinctFiles = (hits: SearchHit[]) => new Set(hits.map((h) => h.path)).size;

interface Options {
  sessionId: string;
  /** False while the panel is closed: aborts any stream. */
  enabled?: boolean;
  /** Test seam; defaults to the Connect streaming client. */
  stream?: SearchStream;
}

/** Debounced, abortable content search: one live stream, request-id guarded, never auto-retried. */
export function useContentSearch({ sessionId, enabled = true, stream }: Options) {
  const analytics = useFilesAnalytics({ component: "SearchPanel", sessionId });
  const { onFlagOff } = useFilesFlags();
  const clientId = useRef<string>("");
  if (!clientId.current) clientId.current = newClientId();
  const run = useMemo<SearchStream>(() => stream ?? connectSearchStream(sessionId, clientId.current), [stream, sessionId]);

  const [state, setState] = useState<SearchState>(INITIAL);
  const live = useRef({ state, analytics, run, onFlagOff });
  live.current = { state, analytics, run, onFlagOff };
  const requestId = useRef(0);
  const controller = useRef<AbortController | null>(null);
  const debounce = useRef<ReturnType<typeof setTimeout> | null>(null);
  const hiddenTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastQuery = useRef<SearchQuery | null>(null);
  const mounted = useRef(true);

  const abort = useCallback(() => {
    controller.current?.abort();
    controller.current = null;
    requestId.current += 1;
  }, []);
  const clearDebounce = useCallback(() => {
    if (debounce.current !== null) clearTimeout(debounce.current);
    debounce.current = null;
  }, []);

  const start = useCallback((q: SearchQuery) => {
    abort();
    const id = requestId.current;
    const ctl = new AbortController();
    controller.current = ctl;
    lastQuery.current = q;
    const startedAt = now();
    const { analytics: a } = live.current;
    a.emit("files.search.run", {
      query_len: bucketQueryLen(q.pattern.length),
      opts: searchOpts({ regex: q.regex, caseSensitive: q.caseSensitive, wholeWord: q.wholeWord, globs: q.include.length + q.exclude.length > 0, ignored: q.includeIgnored, large: q.largeFiles }),
    });
    setState((s) => ({ ...s, status: "searching", ahead: 0, message: null, stale: s.hits.length > 0 }));

    let received: SearchHit[] = [];
    let firstMs: number | null = null;
    let queued = false;
    const finish = (outcome: Outcome, stats: Stats | null) => {
      const scale = bucketScale(stats?.filesScanned ?? 0);
      const labels = { results: bucketResults(received.length), files: bucketFiles(distinctFiles(received)), queued: String(queued) as "true" | "false" };
      a.emit("files.search.results", { outcome, ...labels });
      if (firstMs !== null) a.emitTimed("files.perf.search_first_result_ms", firstMs, { scale, queued: labels.queued });
      const total = outcome === "ok" || outcome === "truncated" ? outcome : outcome === "empty" ? "ok" : "error";
      a.emitTimed("files.perf.search_total_ms", now() - startedAt, { outcome: total, scale });
    };
    const isLive = () => requestId.current === id && !ctl.signal.aborted && mounted.current;

    void (async () => {
      try {
        for await (const ev of live.current.run(q, ctl.signal)) {
          if (!isLive()) return;
          if (ev.type === "queued") {
            queued = true;
            a.emit("files.search.queued", { ahead: ev.ahead >= 2 ? "2+" : "1" });
            setState((s) => ({ ...s, status: "queued", ahead: ev.ahead }));
          } else if (ev.type === "batch") {
            const first = received.length === 0;
            if (firstMs === null) firstMs = now() - startedAt;
            received = received.concat(ev.matches);
            setState((s) => ({ ...s, status: "searching", ahead: 0, hits: first ? ev.matches : s.hits.concat(ev.matches), stale: false, answered: q }));
          } else {
            const hits = received;
            setState((s) => ({ ...s, status: "done", ahead: 0, hits, stale: false, stats: ev, answered: q }));
            finish(ev.truncated ? "truncated" : hits.length === 0 ? "empty" : "ok", ev);
          }
        }
      } catch (err) {
        if (!isLive()) return;
        const { kind, message } = classifySearchError(err);
        if (kind === "cancelled") return;
        if (kind === "flag_off") void live.current.onFlagOff();
        const status: SearchStatus = kind === "worktree" ? "worktree_unavailable" : kind === "flag_off" ? "disabled" : kind;
        setState((s) => ({ ...s, status, ahead: 0, message: message ?? null }));
        finish(kind === "invalid" ? "invalid_regex" : kind === "worktree" ? "worktree_unavailable" : kind === "busy" ? "busy" : kind === "flag_off" ? "flag_off" : "error", null);
      }
    })();
  }, [abort]);

  const submit = useCallback((q: SearchQuery) => {
    if (q.pattern.length < MIN_QUERY_LENGTH) {
      abort();
      setState((s) => ({ ...s, status: "idle", ahead: 0, stale: s.hits.length > 0 }));
      return;
    }
    start(q);
  }, [abort, start]);

  /** Typing: coalesces into one request 300 ms after the last change. */
  const search = useCallback((q: SearchQuery) => {
    clearDebounce();
    debounce.current = setTimeout(() => { debounce.current = null; submit(q); }, SEARCH_DEBOUNCE_MS);
  }, [clearDebounce, submit]);

  /** Enter or a toggle: no debounce. */
  const runNow = useCallback((q: SearchQuery) => {
    clearDebounce();
    submit(q);
  }, [clearDebounce, submit]);

  const retry = useCallback(() => {
    if (lastQuery.current) runNow(lastQuery.current);
  }, [runNow]);

  const cancel = useCallback(() => {
    clearDebounce();
    const { state: s, analytics: a } = live.current;
    if (s.status !== "queued" && s.status !== "searching") return;
    a.emit("files.search.cancel", { phase: s.status === "queued" ? "queued" : "running", results: bucketResults(s.hits.length) });
    abort();
    // A queued search never produced anything of its own, so it returns to idle with the prior results.
    setState((cur) => ({ ...cur, status: cur.status === "queued" ? "idle" : "cancelled", ahead: 0 }));
  }, [abort, clearDebounce]);

  const clear = useCallback(() => {
    clearDebounce();
    abort();
    lastQuery.current = null;
    setState(INITIAL);
  }, [abort, clearDebounce]);

  // Panel closed or unmounted: nothing keeps a connection open.
  useEffect(() => {
    mounted.current = true;
    if (!enabled) {
      clearDebounce();
      abort();
      // A search cut off by closing the panel keeps its partial results and stops claiming to run.
      setState((cur) => (cur.status === "queued" || cur.status === "searching" ? { ...cur, status: "idle", ahead: 0, stale: cur.hits.length > 0 } : cur));
    }
    return () => {
      mounted.current = false;
    };
  }, [enabled, abort, clearDebounce]);
  useEffect(() => () => { clearDebounce(); abort(); }, [abort, clearDebounce]);

  // A hidden document releases its connection after 30 s; the user re-runs on return.
  useEffect(() => {
    const onVisibility = () => {
      if (hiddenTimer.current !== null) clearTimeout(hiddenTimer.current);
      hiddenTimer.current = null;
      if (document.visibilityState !== "hidden") return;
      hiddenTimer.current = setTimeout(() => {
        hiddenTimer.current = null;
        const st = live.current.state.status;
        if (st !== "queued" && st !== "searching") return;
        abort();
        setState((cur) => ({ ...cur, status: "cancelled", ahead: 0 }));
      }, HIDDEN_ABORT_MS);
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      if (hiddenTimer.current !== null) clearTimeout(hiddenTimer.current);
    };
  }, [abort]);

  return { ...state, search, runNow, retry, cancel, clear, lastQuery: lastQuery.current };
}

export type ContentSearch = ReturnType<typeof useContentSearch>;
