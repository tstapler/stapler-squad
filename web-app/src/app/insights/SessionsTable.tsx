// +feature: insights-dashboard
"use client";

import React, { useState, useMemo, useCallback } from "react";
import { TableVirtuoso } from "react-virtuoso";
import Fuse from "fuse.js";
import type { SessionTokenSummary } from "@/gen/session/v1/insights_pb";
import type { BacklogIndexEntry } from "@/lib/hooks/useBacklogService";
import {
  tableCard,
  tableHeader,
  tableTitle,
  orphanToggle,
  table,
  th,
  thRight,
  td,
  tdRight,
  tdMono,
  orphanBadge,
  backlogBadge,
  unpricedBadge,
  empty,
  filterBar,
  searchInput,
  modelSelect,
  clearButton,
  virtualContainer,
  clickableRow,
  sortableTh,
  sortOrderHint,
} from "./SessionsTable.css";
import { fmtCost, fmtTokens, fmtPct, shortId } from "./insightsFormatters";

interface Props {
  sessions: SessionTokenSummary[];
  onSessionClick?: (session: SessionTokenSummary) => void;
  backlogIndex?: Map<string, BacklogIndexEntry>;
}

function pathBasename(p: string): string {
  return p.split("/").pop() || p;
}

// sessionDurationSeconds returns lastMessageAt - firstMessageAt in seconds,
// or 0 when either timestamp is missing (documented decision: unlike cost, a
// missing duration isn't "bad," so it sorts at its natural numeric position
// rather than being pushed to either end — see SortColumn's "duration" case).
function sessionDurationSeconds(s: SessionTokenSummary): number {
  if (!s.firstMessageAt || !s.lastMessageAt) return 0;
  return Number(s.lastMessageAt.seconds) - Number(s.firstMessageAt.seconds);
}

function fmtDuration(seconds: number): string {
  if (seconds <= 0) return "—";
  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  if (mins === 0) return `${secs}s`;
  return `${mins}m ${secs}s`;
}

function fmtSignedCost(usd: number): string {
  const sign = usd < 0 ? "-" : "+";
  return `${sign}$${Math.abs(usd).toFixed(2)}`;
}

const VIRTUOSO_THRESHOLD = 50;

type SortColumn =
  | "input"
  | "output"
  | "cache"
  | "cost"
  | "duration"
  | "costPerMessage"
  | "cacheRoi"
  | "wasteScore";

export function SessionsTable({ sessions, onSessionClick, backlogIndex }: Props) {
  const [showOrphans, setShowOrphans] = useState(true);
  const [searchText, setSearchText] = useState("");
  const [modelFilter, setModelFilter] = useState("");
  const [sortCol, setSortCol] = useState<SortColumn | null>(null);
  const [sortAsc, setSortAsc] = useState(false);

  const orphanCount = sessions.filter((s) => s.isOrphan).length;

  // Build fuse documents that pair each session with its backlog title for searching.
  type FuseDoc = { session: SessionTokenSummary; backlogTitle: string };
  const fuseDocs = useMemo<FuseDoc[]>(
    () =>
      sessions.map((s) => ({
        session: s,
        backlogTitle: backlogIndex?.get(s.sessionId)?.itemTitle ?? "",
      })),
    [sessions, backlogIndex]
  );

  const fuse = useMemo(
    () =>
      new Fuse(fuseDocs, {
        keys: ["session.projectPath", "backlogTitle"],
        threshold: 0.4,
      }),
    [fuseDocs]
  );

  const uniqueModels = useMemo(() => {
    const seen = new Set<string>();
    for (const s of sessions) {
      if (s.primaryModel) seen.add(s.primaryModel);
    }
    return Array.from(seen).sort();
  }, [sessions]);

  const displayed = useMemo(() => {
    let result: SessionTokenSummary[];

    if (searchText.trim()) {
      result = fuse.search(searchText).map((r) => r.item.session);
    } else {
      result = sessions;
    }

    if (modelFilter) {
      result = result.filter((s) => s.primaryModel === modelFilter);
    }

    if (!showOrphans) {
      result = result.filter((s) => !s.isOrphan);
    }

    if (sortCol === null) {
      // Default (no header clicked yet): unchanged lastMessageAt-desc order.
      return [...result].sort((a, b) => {
        const at = a.lastMessageAt ? Number(a.lastMessageAt.seconds) : 0;
        const bt = b.lastMessageAt ? Number(b.lastMessageAt.seconds) : 0;
        return bt - at;
      });
    }

    return [...result].sort((a, b) => {
      if (sortCol === "cost") {
        // Unpriced sessions always sort last, in both directions — the
        // early-return happens before the sortAsc flip below.
        const aUnpriced = a.unpricedModels.length > 0;
        const bUnpriced = b.unpricedModels.length > 0;
        if (aUnpriced !== bUnpriced) return aUnpriced ? 1 : -1;
        const cmp = a.estimatedCostUsd - b.estimatedCostUsd;
        return sortAsc ? cmp : -cmp;
      }
      if (sortCol === "costPerMessage") {
        // messageCount === 0 sorts last regardless of direction — mirrors
        // the "cost" unpriced-last guard above, avoiding a raw N/0 division
        // that would produce NaN/Infinity.
        const aZero = a.messageCount === 0;
        const bZero = b.messageCount === 0;
        if (aZero !== bZero) return aZero ? 1 : -1;
        if (aZero && bZero) return 0;
        const cmp = a.estimatedCostUsd / a.messageCount - b.estimatedCostUsd / b.messageCount;
        return sortAsc ? cmp : -cmp;
      }
      if (sortCol === "cacheRoi") {
        // Unpriced sessions (ROI undefined) always sort last — same guard
        // shape as "cost". Negative ROI values are real data and sort
        // normally alongside positive ones.
        const aUnpriced = a.unpricedModels.length > 0;
        const bUnpriced = b.unpricedModels.length > 0;
        if (aUnpriced !== bUnpriced) return aUnpriced ? 1 : -1;
        const cmp = a.cacheRoiUsd - b.cacheRoiUsd;
        return sortAsc ? cmp : -cmp;
      }
      if (sortCol === "wasteScore") {
        // Sort-last bucket covers both "unpriced" and "not evaluated"
        // (wasteScore undefined) — which of the two a given row is doesn't
        // affect sort order, only its cell text (see renderCells).
        const aMissing = a.unpricedModels.length > 0 || a.wasteScore === undefined;
        const bMissing = b.unpricedModels.length > 0 || b.wasteScore === undefined;
        if (aMissing !== bMissing) return aMissing ? 1 : -1;
        if (aMissing && bMissing) return 0;
        const cmp = (a.wasteScore as number) - (b.wasteScore as number);
        return sortAsc ? cmp : -cmp;
      }
      let cmp = 0;
      switch (sortCol) {
        case "input":
          cmp = Number(a.totalInputTokens - b.totalInputTokens);
          break;
        case "output":
          cmp = Number(a.totalOutputTokens - b.totalOutputTokens);
          break;
        case "cache":
          cmp = a.cacheHitRate - b.cacheHitRate;
          break;
        case "duration":
          // Missing timestamps default to 0 — a missing duration isn't
          // "bad" like a missing price, so it sorts at its natural numeric
          // position rather than being pushed to either end.
          cmp = sessionDurationSeconds(a) - sessionDurationSeconds(b);
          break;
      }
      return sortAsc ? cmp : -cmp;
    });
  }, [sessions, searchText, modelFilter, showOrphans, fuse, sortCol, sortAsc]);

  const handleSortClick = useCallback((col: SortColumn) => {
    // Reads sortCol from closure rather than nesting setSortAsc inside
    // setSortCol's updater — a state setter called as a side effect of
    // another setter's functional update breaks under React.StrictMode's
    // double-invocation of updater functions (the toggle would fire twice
    // and cancel out). Mirrors app/backlog/page.tsx's independent-calls
    // precedent for the same sort-toggle shape.
    if (sortCol === col) {
      setSortAsc((prevAsc) => !prevAsc);
    } else {
      setSortCol(col);
      setSortAsc(false);
    }
  }, [sortCol]);

  const sortIndicator = useCallback(
    (col: SortColumn) => (sortCol === col ? (sortAsc ? " ↑" : " ↓") : " ↕"),
    [sortCol, sortAsc]
  );

  const hasActiveFilters = searchText !== "" || modelFilter !== "";

  function clearFilters() {
    setSearchText("");
    setModelFilter("");
  }

  const handleRowKeyDown = useCallback((e: React.KeyboardEvent<HTMLTableRowElement>, s: SessionTokenSummary) => {
    if ((e.key === "Enter" || e.key === " ") && onSessionClick) {
      e.preventDefault();
      onSessionClick(s);
    }
  }, [onSessionClick]);

  const sortableHeaderCell = (col: SortColumn, label: string) => (
    <th
      className={thRight}
      scope="col"
      aria-sort={sortCol === col ? (sortAsc ? "ascending" : "descending") : "none"}
    >
      <span
        className={sortableTh}
        role="button"
        tabIndex={0}
        onClick={() => handleSortClick(col)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            handleSortClick(col);
          }
        }}
      >
        {label}
        {sortIndicator(col)}
      </span>
    </th>
  );

  const headerContent = () => (
    <tr>
      <th className={th} scope="col">Session</th>
      <th className={th} scope="col">Model</th>
      <th className={th} scope="col">Path</th>
      {sortableHeaderCell("input", "Input")}
      {sortableHeaderCell("output", "Output")}
      {sortableHeaderCell("cache", "Cache")}
      {sortableHeaderCell("cost", "Cost")}
      {sortableHeaderCell("duration", "Duration")}
      {sortableHeaderCell("costPerMessage", "Cost/Msg")}
      {sortableHeaderCell("cacheRoi", "Cache ROI")}
      {sortableHeaderCell("wasteScore", "Waste Score")}
    </tr>
  );

  const renderCells = (_index: number, s: SessionTokenSummary) => {
    const backlogEntry = backlogIndex?.get(s.sessionId);
    return (
      <>
        <td className={tdMono} title={s.sessionId || s.conversationId}>
          {s.isOrphan ? (
            <>
              {shortId(s.conversationId)}
              <span className={orphanBadge}>orphan</span>
            </>
          ) : (
            shortId(s.sessionId || s.conversationId)
          )}
          {backlogEntry && (
            <a
              href={`/backlog?item=${backlogEntry.itemId}`}
              className={backlogBadge}
              data-testid="backlog-badge"
              title={`${backlogEntry.sessionRole}: ${backlogEntry.itemTitle}`}
              onClick={(e) => e.stopPropagation()}
            >
              {backlogEntry.sessionRole}: {backlogEntry.itemTitle}
            </a>
          )}
        </td>
        <td className={td} title={s.primaryModel}>{s.primaryModel || "—"}</td>
        <td className={td} title={s.projectPath}>{pathBasename(s.projectPath) || "—"}</td>
        <td className={tdRight}>{fmtTokens(s.totalInputTokens)}</td>
        <td className={tdRight}>{fmtTokens(s.totalOutputTokens)}</td>
        <td className={tdRight} title={`${fmtTokens(s.cacheReadTokens)} read, ${fmtTokens(s.cacheCreationTokens)} written`}>
          {fmtPct(s.cacheHitRate)}
        </td>
        <td className={tdRight}>
          {fmtCost(s.estimatedCostUsd)}
          {s.unpricedModels.length > 0 && <span className={unpricedBadge}>unpriced</span>}
        </td>
        <td className={tdRight}>{fmtDuration(sessionDurationSeconds(s))}</td>
        <td className={tdRight}>
          {s.messageCount === 0 ? "Not evaluated" : fmtCost(s.estimatedCostUsd / s.messageCount)}
        </td>
        <td className={tdRight}>
          {s.unpricedModels.length > 0 ? (
            <span className={unpricedBadge}>unpriced</span>
          ) : (
            fmtSignedCost(s.cacheRoiUsd)
          )}
        </td>
        <td className={tdRight}>
          {s.unpricedModels.length > 0
            ? "—"
            : s.wasteScore === undefined
              ? "Not evaluated"
              : s.wasteScore}
        </td>
      </>
    );
  };

  const virtuosoComponents = useMemo(() => ({
    Table: ({ style: s, ...props }: React.ComponentPropsWithRef<"table">) => (
      <table className={table} style={s} {...props} />
    ),
    // eslint-disable-next-line react/display-name
    TableHead: React.forwardRef<HTMLTableSectionElement, React.ComponentPropsWithRef<"thead">>(
      (props, ref) => <thead ref={ref} {...props} />
    ),
    // eslint-disable-next-line react/display-name
    TableBody: React.forwardRef<HTMLTableSectionElement, React.ComponentPropsWithRef<"tbody">>(
      (props, ref) => <tbody ref={ref} {...props} />
    ),
    TableRow: ({ "data-index": dataIndex, ...props }: React.ComponentPropsWithRef<"tr"> & { "data-index": number }) => {
      const s = displayed[dataIndex];
      return (
        <tr
          data-index={dataIndex}
          {...props}
          className={onSessionClick ? clickableRow : undefined}
          onClick={onSessionClick && s ? () => onSessionClick(s) : undefined}
          onKeyDown={s ? (e) => handleRowKeyDown(e as React.KeyboardEvent<HTMLTableRowElement>, s) : undefined}
          tabIndex={onSessionClick ? 0 : undefined}
          role={onSessionClick ? "button" : undefined}
        />
      );
    },
  }), [displayed, onSessionClick, handleRowKeyDown]);

  const titleText = hasActiveFilters
    ? `Sessions (${displayed.length} of ${sessions.length})`
    : `Sessions (${displayed.length})`;

  return (
    <div className={tableCard}>
      <div className={tableHeader}>
        <div className={tableTitle}>
          {titleText}
          {sortCol === null && (
            <span className={sortOrderHint}> — sorted by most recently active</span>
          )}
        </div>
        <div className={filterBar}>
          <input
            type="search"
            className={searchInput}
            placeholder="Search by path…"
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            aria-label="Search sessions by project path"
          />
          <select
            className={modelSelect}
            value={modelFilter}
            onChange={(e) => setModelFilter(e.target.value)}
            aria-label="Filter by model"
          >
            <option value="">All models</option>
            {uniqueModels.map((m) => (
              <option key={m} value={m}>{m}</option>
            ))}
          </select>
          {hasActiveFilters && (
            <button
              type="button"
              className={clearButton}
              onClick={clearFilters}
            >
              Clear filters
            </button>
          )}
          {orphanCount > 0 && (
            <button
              type="button"
              className={orphanToggle}
              onClick={() => setShowOrphans((v) => !v)}
            >
              {showOrphans ? "Hide" : "Show"} orphans ({orphanCount})
            </button>
          )}
        </div>
      </div>

      {displayed.length === 0 ? (
        <div className={empty}>
          {hasActiveFilters ? "No sessions match your filters" : "No sessions"}
        </div>
      ) : displayed.length > VIRTUOSO_THRESHOLD ? (
        <div className={virtualContainer} data-testid="virtuoso-table">
          <TableVirtuoso
            data={displayed}
            fixedHeaderContent={headerContent}
            itemContent={renderCells}
            style={{ height: "100%" }}
            components={virtuosoComponents}
          />
        </div>
      ) : (
        <table className={table}>
          <thead>{headerContent()}</thead>
          <tbody>
            {displayed.map((s, i) => (
              <tr
                key={s.conversationId || s.sessionId}
                className={onSessionClick ? clickableRow : undefined}
                onClick={onSessionClick ? () => onSessionClick(s) : undefined}
                onKeyDown={(e) => handleRowKeyDown(e, s)}
                tabIndex={onSessionClick ? 0 : undefined}
                role={onSessionClick ? "button" : undefined}
              >
                {renderCells(i, s)}
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
