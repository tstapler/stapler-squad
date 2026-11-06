"use client";

// +feature: session-artifacts-browse

import { useEffect, useMemo, useRef } from "react";
import { Link, useReplaceSearch, useRouter, useSearchParams } from "@/lib/nav";
import { routes } from "@/lib/routes";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { useMediaQuery } from "@/lib/hooks/useMediaQuery";
import { useNotificationCommands } from "@/lib/contexts/NotificationContext";
import { useArtifactsData } from "@/lib/hooks/useArtifactsData";
import { activeFilterCount, modeOf, parseBrowseParams, parsePrParam, prLabelOf, toSearch, type BrowseParams } from "@/lib/artifacts/browseQuery";
import { ArtifactResults } from "./ArtifactResults";
import { ArtifactsFilters } from "./ArtifactsFilters";
import { ArtifactsSearchBox } from "./ArtifactsSearchBox";
import { itemsFromHits, itemsFromRows, sourceLabelOf } from "./artifactDisplay";
import * as s from "./ArtifactsBrowser.css";

const COMPACT_QUERY = "(max-width: 767px)";

function resultCount(mode: string, shown: number, total: number): string {
  if (mode === "browse") return `Showing ${shown} of ${total}`;
  return `${total} ${total === 1 ? "session" : "sessions"} found`;
}

function unscannedLine(n: number): string {
  return `${n} ${n === 1 ? "session isn't" : "sessions aren't"} scanned yet, so results may be incomplete`;
}

/** The /artifacts page body: browse, filter and search artifacts across sessions (design/ux.md S8, S9). */
export function ArtifactsBrowser() {
  const router = useRouter();
  const replaceSearch = useReplaceSearch();
  const searchParams = useSearchParams();
  const { announce } = useAnnounce();
  const { addNotification } = useNotificationCommands();
  const compact = useMediaQuery(COMPACT_QUERY);

  const params = useMemo(() => parseBrowseParams(searchParams), [searchParams]);
  const mode = modeOf(params);
  const prTarget = params.pr ? parsePrParam(params.pr) : null;
  const prInvalid = Boolean(params.pr) && !prTarget;
  const state = useArtifactsData(params, !prInvalid);
  const { data, loading, slow, error } = state;

  const go = (next: BrowseParams) => replaceSearch(toSearch({ ...next, view: "" }));
  const patch = (p: Partial<BrowseParams>) => go({ ...params, ...p });
  const clearFilters = () => go({ ...params, type: "", repo: "", state: "" });
  const clearSearch = () => go({ ...params, q: "", pr: "" });

  // S9: an exact PR lookup with one match opens that session, with a View all way back to the list.
  const redirected = useRef("");
  useEffect(() => {
    if (mode !== "pr" || params.view === "all" || loading || error || !prTarget) return;
    if (data?.mode !== "pr" || data.hits.length !== 1) return;
    const hit = data.hits[0];
    const key = `${params.pr}:${hit.sessionId}`;
    if (redirected.current === key) return;
    redirected.current = key;
    router.replace(routes.sessionDetail(hit.sessionId));
    addNotification({
      sessionId: "",
      sessionName: "",
      title: `Opened the session tracking ${prLabelOf(prTarget)} (${sourceLabelOf(hit.source)})`,
      message: "1 match.",
      notificationType: "info",
      viewLabel: "View all",
      onView: () => router.push(`/artifacts?${toSearch({ ...params, view: "all" })}`),
    });
  }, [mode, params, loading, error, data, prTarget, router, addNotification]);

  const items = useMemo(() => (!data ? [] : data.mode === "browse" ? itemsFromRows(data.rows) : itemsFromHits(data.hits)), [data]);
  const total = data?.totalCount ?? 0;

  useEffect(() => {
    if (loading && !data) announce("Loading artifacts", "polite", "artifacts-count");
    else if (!loading && !error && data) announce(resultCount(data.mode, items.length, total), "polite", "artifacts-count");
  }, [loading, error, data, items.length, total, announce]);

  const searchLabel = mode === "pr" && prTarget ? prLabelOf(prTarget) : params.q;
  const filtersActive = activeFilterCount(params) > 0;

  return (
    <main className={s.page} data-testid="artifacts-page" aria-busy={loading}>
      <h1 className={s.heading}>Artifacts</h1>
      <ArtifactsSearchBox value={params.q} slow={slow && mode === "search"} onSearch={(q) => patch({ q })} onClear={() => go({ ...params, q: "" })} />
      {mode === "browse" && <ArtifactsFilters params={params} compact={compact} onChange={patch} onClear={clearFilters} />}
      {mode === "pr" && prTarget && (
        <p className={s.muted} data-testid="artifacts-pr-lookup">
          Sessions tied to {prLabelOf(prTarget)}. <button type="button" className={s.button} onClick={clearSearch}>{/* analytics-exempt */}Clear search</button>
        </p>
      )}
      {prInvalid && (
        <div className={s.errorBox} role="alert" data-testid="artifacts-pr-invalid">
          <span>&ldquo;{params.pr}&rdquo; isn&apos;t a pull request reference. Use owner/repo#number.</span>
          <button type="button" className={s.button} onClick={clearSearch}>{/* analytics-exempt */}Clear search</button>
        </div>
      )}
      {!error && !prInvalid && data && (
        <p className={s.statusLine} data-testid="artifacts-count">
          {resultCount(data.mode, items.length, total)}
          {slow && <span className={s.spinner} aria-hidden="true" />}
        </p>
      )}
      {data && !error && data.unscanned > 0 && (
        <p className={s.notice} data-testid="artifacts-unscanned">
          {unscannedLine(data.unscanned)}. <Link href={routes.home}>Open sessions to rescan</Link>
        </p>
      )}
      {loading && !data && !prInvalid && (
        <p className={s.statusLine} data-testid="artifacts-loading">Loading artifacts</p>
      )}
      {error && (
        <div className={s.errorBox} role="alert" data-testid="artifacts-error">
          <span>{mode === "browse" ? "Couldn't load artifacts" : "Search failed. Browse still works."}</span>
          <button type="button" className={s.button} onClick={state.retry} data-testid="artifacts-retry">{/* analytics-exempt */}Retry</button>
          {mode !== "browse" && <button type="button" className={s.button} onClick={clearSearch}>{/* analytics-exempt */}Clear search</button>}
        </div>
      )}
      {!error && data && items.length === 0 && !loading && (
        <div className={s.empty} data-testid="artifacts-empty">
          {mode === "browse" && filtersActive && (<><p className={s.muted}>No artifacts match these filters.</p><button type="button" className={s.button} onClick={clearFilters}>{/* analytics-exempt */}Clear filters</button></>)}
          {mode === "browse" && !filtersActive && (<><p className={s.muted}>No artifacts yet. Artifacts appear as sessions open PRs, make commits and write plans.</p><Link href={routes.home} className={s.sessionLink}>Go to sessions</Link></>)}
          {mode !== "browse" && (<><p className={s.muted}>No sessions found for &quot;{searchLabel}&quot;.</p><p className={s.hint}>Try the PR number (acme/app#12), a branch name, or a SHA prefix of at least 7 characters</p><button type="button" className={s.button} onClick={clearSearch}>{/* analytics-exempt */}Clear search</button></>)}
        </div>
      )}
      {!error && items.length > 0 && <ArtifactResults items={items} compact={compact} dimmed={loading} />}
      {!error && data?.nextPageToken && (
        <button type="button" className={`${s.button} ${s.loadMore}`} onClick={state.loadMore} disabled={state.loadingMore} data-testid="artifacts-load-more">{/* analytics-exempt */}
          {state.loadingMore ? "Loading" : "Load more"}
        </button>
      )}
      {state.loadMoreError && (
        <p className={s.invalid} data-testid="artifacts-load-more-error">Couldn&apos;t load more. <button type="button" className={s.button} onClick={state.loadMore}>{/* analytics-exempt */}Retry</button></p>
      )}
    </main>
  );
}

