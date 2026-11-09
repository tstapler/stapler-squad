"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { SessionArtifactHit, SessionArtifactRow } from "@/gen/session/v1/session_pb";
import { artifactsApi } from "@/lib/artifacts/artifactsApi";
import { modeOf, parsePrParam, toSearch, type BrowseMode, type BrowseParams } from "@/lib/artifacts/browseQuery";

/** UX-S8 "Initial loading": a request this slow becomes the error state. */
export const LOAD_TIMEOUT_MS = 15_000;
/** UX-S8 "Slow search": past this the previous results dim and a spinner shows. */
export const SLOW_AFTER_MS = 1_000;

/** Abort reason for a request a newer one replaced; any other abort reason is the 15 s timeout. */
const SUPERSEDED = "superseded";

export interface ArtifactsData {
  mode: BrowseMode;
  rows: SessionArtifactRow[];
  hits: SessionArtifactHit[];
  totalCount: number;
  nextPageToken: string;
  unscanned: number;
}

const EMPTY: ArtifactsData = { mode: "browse", rows: [], hits: [], totalCount: 0, nextPageToken: "", unscanned: 0 };

export interface ArtifactsState {
  /** Null until the first response for the current view arrives. */
  data: ArtifactsData | null;
  loading: boolean;
  slow: boolean;
  error: boolean;
  loadingMore: boolean;
  loadMoreError: boolean;
  retry: () => void;
  loadMore: () => void;
}

/** Everything a request depends on; `view` only switches the single-match shortcut and is left out. */
export function requestKey(p: BrowseParams): string {
  return toSearch({ ...p, view: "" });
}

export function useArtifactsData(params: BrowseParams, enabled: boolean): ArtifactsState {
  const key = requestKey(params);
  const paramsRef = useRef(params);
  paramsRef.current = params;

  const [data, setData] = useState<ArtifactsData | null>(null);
  const [loading, setLoading] = useState(enabled);
  const [slow, setSlow] = useState(false);
  const [error, setError] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadMoreError, setLoadMoreError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const dataRef = useRef<ArtifactsData | null>(null);
  dataRef.current = data;

  useEffect(() => {
    if (!enabled) { setLoading(false); return; }
    const p = paramsRef.current;
    const mode = modeOf(p);
    const pr = p.pr ? parsePrParam(p.pr) : null;
    const ctrl = new AbortController();
    const slowTimer = setTimeout(() => setSlow(true), SLOW_AFTER_MS);
    const timeout = setTimeout(() => ctrl.abort(), LOAD_TIMEOUT_MS);
    setLoading(true);
    setError(false);
    setLoadMoreError(false);

    const run = async (): Promise<ArtifactsData> => {
      if (mode === "browse") {
        const page = await artifactsApi.browse(p, "", ctrl.signal);
        return { ...EMPTY, mode, rows: page.rows, totalCount: page.totalCount, nextPageToken: page.nextPageToken, unscanned: page.unscanned };
      }
      const list = mode === "pr" && pr ? await artifactsApi.findByPr(pr, ctrl.signal) : await artifactsApi.search(p, pr, ctrl.signal);
      return { ...EMPTY, mode, hits: list.hits, totalCount: list.hits.length, unscanned: list.unscanned };
    };

    const isCurrent = () => ctrl.signal.reason !== SUPERSEDED;
    run().then(
      (next) => { if (isCurrent()) setData(next); },
      () => { if (isCurrent()) setError(true); },
    ).finally(() => {
      clearTimeout(slowTimer);
      clearTimeout(timeout);
      if (!isCurrent()) return;
      setSlow(false);
      setLoading(false);
    });

    return () => {
      ctrl.abort(SUPERSEDED);
      clearTimeout(slowTimer);
      clearTimeout(timeout);
    };
  }, [key, enabled, attempt]);

  const loadMore = useCallback(() => {
    const current = dataRef.current;
    if (!current?.nextPageToken) return;
    setLoadingMore(true);
    setLoadMoreError(false);
    const ctrl = new AbortController();
    artifactsApi.browse(paramsRef.current, current.nextPageToken, ctrl.signal).then(
      (page) => setData((prev) => (prev ? { ...prev, rows: [...prev.rows, ...page.rows], nextPageToken: page.nextPageToken, totalCount: page.totalCount } : prev)),
      () => setLoadMoreError(true),
    ).finally(() => setLoadingMore(false));
  }, []);

  const retry = useCallback(() => setAttempt((n) => n + 1), []);

  return { data, loading, slow, error, loadingMore, loadMoreError, retry, loadMore };
}
