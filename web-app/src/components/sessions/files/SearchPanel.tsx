"use client";

import {
  forwardRef, useCallback, useEffect, useId, useImperativeHandle, useMemo, useRef, useState, type KeyboardEvent,
} from "react";
import { bucketInteractions, bucketRank } from "@/lib/files/analytics/events";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";
import { FindFlowMeter } from "@/lib/files/analytics/findFlowMeter";
import type { SearchHit, SearchQuery, SearchStream } from "@/lib/files/searchClient";
import { useCoarsePointer } from "@/lib/hooks/useCoarsePointer";
import { useContentSearch } from "@/lib/hooks/useContentSearch";
import { usePendingDelay } from "./PendingState";
import { SearchResults, rowDomId, type ActivateVia, type SearchResultsHandle } from "./SearchResults";
import { buildRows, hitKey } from "./searchRows";
import {
  checkLabel, emptyMessage, fieldLabel, filters, form as formClass, header, headerTab, helper, inlineError, note, panel,
  queryInput, queryRow, status as statusClass, textButton, toggle, toggleRow,
} from "./SearchPanel.css";

export type LeftPane = "explorer" | "search";

/** The Explorer | Search header above the left pane. */
export function SearchPaneTabs({ pane, onPane }: { pane: LeftPane; onPane: (pane: LeftPane) => void }) {
  return (
    <div className={header} role="tablist" aria-label="Left panel">
      {(["explorer", "search"] as const).map((id) => (
        <button
          key={id}
          type="button"
          role="tab"
          id={`files-left-${id}`}
          aria-selected={pane === id}
          data-testid={`files-left-${id}`}
          className={headerTab}
          onClick={() => onPane(id)}
        >
          {id === "explorer" ? "Explorer" : "Search"}
        </button>
      ))}
    </div>
  );
}

interface FormState {
  pattern: string;
  regex: boolean;
  caseSensitive: boolean;
  wholeWord: boolean;
  include: string;
  exclude: string;
  includeIgnored: boolean;
}
const EMPTY: FormState = { pattern: "", regex: false, caseSensitive: false, wholeWord: false, include: "", exclude: "", includeIgnored: false };
const globs = (raw: string) => raw.split(",").map((g) => g.trim()).filter(Boolean);
const toQuery = (f: FormState): SearchQuery => ({
  pattern: f.pattern, regex: f.regex, caseSensitive: f.caseSensitive, wholeWord: f.wholeWord,
  include: globs(f.include), exclude: globs(f.exclude), includeIgnored: f.includeIgnored, largeFiles: false,
});

/** "(case-sensitive, regex)" style list of the toggles that are on. */
function activeToggles(q: SearchQuery): string {
  const on = [q.caseSensitive && "case-sensitive", q.wholeWord && "whole word", q.regex && "regex", q.includeIgnored && "ignored files included"].filter(Boolean);
  return on.length ? ` (${on.join(", ")})` : "";
}

export interface SearchPanelHandle {
  focusQuery(): void;
}

interface SearchPanelProps {
  sessionId: string;
  sessionName?: string;
  /** False while the Explorer shows: aborts any stream but keeps query and results. */
  visible: boolean;
  /** Bumped by Ctrl+Shift+F and the Search tab: moves focus to the query field. */
  focusToken: number;
  dirtyPaths: ReadonlySet<string>;
  /** Opens a hit in a preview tab; returns whether an existing tab was reused. */
  openHit: (hit: SearchHit, via: ActivateVia) => boolean;
  /** Esc: focus goes back to the editor. */
  onEscape: () => void;
  onResumeSession?: () => void;
  /** Test seam. */
  stream?: SearchStream;
}

function resultsText(count: number, files: number): string {
  return `${count} ${count === 1 ? "result" : "results"} in ${files} ${files === 1 ? "file" : "files"}`;
}

/** Query, toggles, filters and the grouped results of the content search. */
export const SearchPanel = forwardRef<SearchPanelHandle, SearchPanelProps>(function SearchPanel(p, ref) {
  const analytics = useFilesAnalytics({ component: "SearchPanel", sessionId: p.sessionId });
  const search = useContentSearch({ sessionId: p.sessionId, enabled: p.visible, stream: p.stream });
  const coarse = useCoarsePointer();
  const treeId = useId();
  const [form, setForm] = useState<FormState>(EMPTY);
  const [showFilters, setShowFilters] = useState(false);
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const [activeKey, setActiveKey] = useState<string | null>(null);
  const [includeFocusRequest, setIncludeFocusRequest] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const includeRef = useRef<HTMLInputElement>(null);
  const resultsRef = useRef<SearchResultsHandle>(null);
  const meter = useRef(new FindFlowMeter());
  const dirtyEdit = useRef(false);

  const rows = useMemo(() => buildRows(search.hits, collapsed), [search.hits, collapsed]);
  const files = useMemo(() => new Set(search.hits.map((h) => h.path)).size, [search.hits]);
  const slow = usePendingDelay(search.status === "searching", 1000);

  useImperativeHandle(ref, () => ({ focusQuery: () => inputRef.current?.focus() }), []);

  // Opening the panel starts a find flow; closing it discards the flow without an event.
  useEffect(() => {
    if (p.visible) meter.current.open();
    else meter.current.cancel();
  }, [p.visible]);
  useEffect(() => {
    if (p.focusToken > 0) inputRef.current?.focus();
  }, [p.focusToken]);
  useEffect(() => {
    setActiveKey(null);
  }, [search.hits]);
  useEffect(() => {
    if (includeFocusRequest > 0) includeRef.current?.focus();
  }, [includeFocusRequest]);
  useEffect(() => {
    if (activeKey) resultsRef.current?.scrollToKey(activeKey);
  }, [activeKey]);

  const change = useCallback((next: FormState, immediate: boolean) => {
    setForm(next);
    dirtyEdit.current = true;
    const q = toQuery(next);
    if (immediate) search.runNow(q);
    else search.search(q);
  }, [search]);

  // One interaction per edit that leads to a request (typing a query is one, not one per key).
  const requested = search.lastQuery;
  useEffect(() => {
    if (requested && dirtyEdit.current) {
      meter.current.interact();
      dirtyEdit.current = false;
    }
  }, [requested]);

  const activate = useCallback((hit: SearchHit, hitIndex: number, via: ActivateVia) => {
    const reused = p.openHit(hit, via);
    const interactions = meter.current.finish();
    if (interactions !== null) {
      analytics.emit("files.search.open_hit", {
        rank: bucketRank(hitIndex + 1), interactions: bucketInteractions(interactions), via, reused_tab: String(reused) as "true" | "false",
      });
    }
    meter.current.open();
  }, [p, analytics]);

  const toggleGroup = useCallback((path: string) => {
    setCollapsed((cur) => {
      const next = new Set(cur);
      if (!next.delete(path)) next.add(path);
      return next;
    });
  }, []);

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Escape") {
      e.preventDefault();
      p.onEscape();
      return;
    }
    if (e.target !== inputRef.current) return;
    const keys = rows.map((r) => r.key);
    const at = activeKey ? keys.indexOf(activeKey) : -1;
    const move = (to: number) => { e.preventDefault(); setActiveKey(keys[Math.max(0, Math.min(keys.length - 1, to))] ?? null); };
    if (e.key === "ArrowDown") move(at + 1);
    else if (e.key === "ArrowUp") move(at - 1);
    else if (e.key === "Home" && e.ctrlKey) move(0);
    else if (e.key === "End" && e.ctrlKey) move(keys.length - 1);
    else if (e.key === "Enter") {
      e.preventDefault();
      const row = rows.find((r) => r.key === activeKey);
      if (row?.kind === "group") toggleGroup(row.path);
      else if (row?.kind === "hit") activate(row.hit, row.hitIndex, "enter");
      else if (search.hits.length > 0 && !activeKey) activate(search.hits[0], 0, "enter");
      else search.runNow(toQuery(form));
    }
  };

  const q = search.answered ?? toQuery(form);
  const typed = form.pattern.length;
  const st = search.status;
  const hasResults = search.hits.length > 0;
  const worktreeGone = st === "worktree_unavailable";
  const busyStream = st === "searching" || st === "queued";

  let line: string | null = null;
  if (st === "queued") line = `Waiting for other searches (${search.ahead} ahead)...`;
  else if (st === "searching") line = slow ? `Searching... ${search.hits.length} results so far` : "Searching...";
  else if (st === "cancelled") line = `Search cancelled - ${search.hits.length} results so far`;
  else if (st === "done" && hasResults) line = resultsText(search.hits.length, files);

  const idleCopy = typed < 2 && !busyStream && !worktreeGone;
  const zero = st === "done" && !hasResults;

  return (
    <div className={panel} data-testid="search-panel" onKeyDown={onKeyDown}>
      <div className={formClass}>
        <div className={queryRow}>
          <input
            ref={inputRef}
            type="search"
            className={queryInput}
            aria-label="Search file contents"
            aria-controls={treeId}
            aria-activedescendant={activeKey ? rowDomId(treeId, activeKey) : undefined}
            inputMode="search"
            autoComplete="off"
            spellCheck={false}
            placeholder="Search"
            value={form.pattern}
            data-testid="search-query"
            onChange={(e) => change({ ...form, pattern: e.target.value }, false)}
          />
        </div>
        <div className={toggleRow}>
          <button type="button" className={toggle} aria-pressed={form.caseSensitive} aria-label="Match case" title="Match case" onClick={() => change({ ...form, caseSensitive: !form.caseSensitive }, true)}>Aa</button>
          <button type="button" className={toggle} aria-pressed={form.wholeWord} aria-label="Match whole word" title="Match whole word" onClick={() => change({ ...form, wholeWord: !form.wholeWord }, true)}>ab</button>
          <button type="button" className={toggle} aria-pressed={form.regex} aria-label="Use regular expression" title="Use regular expression" onClick={() => change({ ...form, regex: !form.regex }, true)}>.*</button>
          <button type="button" className={textButton} aria-expanded={showFilters} onClick={() => setShowFilters((v) => !v)}>Filters</button>
        </div>
        {showFilters && (
          <div className={filters}>
            <label className={fieldLabel} htmlFor={`${treeId}-inc`}>Files to include</label>
            <input id={`${treeId}-inc`} ref={includeRef} className={queryInput} value={form.include} placeholder="**/*.go" onChange={(e) => change({ ...form, include: e.target.value }, false)} />
            <label className={fieldLabel} htmlFor={`${treeId}-exc`}>Files to exclude</label>
            <input id={`${treeId}-exc`} className={queryInput} value={form.exclude} placeholder="vendor/**" onChange={(e) => change({ ...form, exclude: e.target.value }, false)} />
            <label className={checkLabel}>
              <input type="checkbox" checked={form.includeIgnored} onChange={(e) => change({ ...form, includeIgnored: e.target.checked }, true)} />
              Include ignored files
            </label>
          </div>
        )}
      </div>

      <div className={statusClass} role="status" aria-live="polite" data-testid="search-status">
        {line}
        {busyStream && <button type="button" className={textButton} data-testid="search-cancel" onClick={search.cancel}>Cancel</button>}
      </div>

      {st === "invalid" && <div className={inlineError}>Invalid regular expression: {search.message}</div>}
      {st === "error" && (
        <div className={emptyMessage}>
          <span>Search failed - check your connection</span>
          <button type="button" className={textButton} onClick={search.retry}>Retry</button>
        </div>
      )}
      {st === "busy" && (
        <div className={emptyMessage}>
          <span>Too many searches are running - try again in a moment</span>
          <button type="button" className={textButton} onClick={search.retry}>Retry</button>
        </div>
      )}
      {worktreeGone && (
        <div className={emptyMessage}>
          <span>Session paused; worktree unavailable</span>
          {p.onResumeSession && <button type="button" className={textButton} onClick={p.onResumeSession}>Resume session</button>}
        </div>
      )}
      {zero && (
        <div className={emptyMessage}>
          <span>{`No matches for \`${q.pattern}\`${activeToggles(q)}`}</span>
          <button type="button" className={textButton} onClick={() => change({ ...EMPTY, pattern: form.pattern }, true)}>Clear filters</button>
        </div>
      )}
      {st === "done" && search.stats?.truncated && (
        <div className={note}>
          Showing first 1000 - refine your query{" "}
          <button type="button" className={textButton} onClick={() => { setShowFilters(true); setIncludeFocusRequest((n) => n + 1); }}>Add filters</button>
        </div>
      )}
      {idleCopy && !hasResults && (
        <div className={helper} data-testid="search-idle">
          <span>Search file contents in {p.sessionName ?? "this session"}</span>
          <span>Type at least 2 characters</span>
          <span>Aa matches case, ab whole words, .* regular expressions. Ignored and binary files are skipped.</span>
        </div>
      )}
      {hasResults && !worktreeGone && (
        <SearchResults
          ref={resultsRef}
          rows={rows}
          stale={search.stale || idleCopy}
          busy={busyStream}
          activeKey={activeKey}
          dirtyPaths={p.dirtyPaths}
          coarse={coarse}
          treeId={treeId}
          onToggleGroup={toggleGroup}
          onActivate={activate}
        />
      )}
      {st === "done" && search.stats && search.stats.binarySkipped > 0 && (
        <div className={note}>Skipped {search.stats.binarySkipped} binary files</div>
      )}
    </div>
  );
});

export { hitKey };
