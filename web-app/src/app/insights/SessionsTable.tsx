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
  sortableThFocus,
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

const VIRTUOSO_THRESHOLD = 50;

type SortColumn = "input" | "output" | "cache" | "cost";

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
      }
      return sortAsc ? cmp : -cmp;
    });
  }, [sessions, searchText, modelFilter, showOrphans, fuse, sortCol, sortAsc]);

  const handleSortClick = useCallback((col: SortColumn) => {
    setSortCol((prevCol) => {
      if (prevCol === col) {
        setSortAsc((prevAsc) => !prevAsc);
        return col;
      }
      setSortAsc(false);
      return col;
    });
  }, []);

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
      aria-sort={sortCol === col ? (sortAsc ? "ascending" : "descending") : "none"}
    >
      <span
        className={`${sortableTh} ${sortableThFocus}`}
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
      <th className={th}>Session</th>
      <th className={th}>Model</th>
      <th className={th}>Path</th>
      {sortableHeaderCell("input", "Input")}
      {sortableHeaderCell("output", "Output")}
      {sortableHeaderCell("cache", "Cache")}
      {sortableHeaderCell("cost", "Cost")}
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
        <td className={tdRight}>{fmtPct(s.cacheHitRate)}</td>
        <td className={tdRight}>
          {fmtCost(s.estimatedCostUsd)}
          {s.unpricedModels.length > 0 && <span className={unpricedBadge}>unpriced</span>}
        </td>
      </>
    );
  };

  const virtuosoComponents = useMemo(() => ({
    Table: ({ style: s, ...props }: React.ComponentPropsWithRef<"table">) => (
      <table className={table} style={s} {...props} />
    ),
    TableHead: (props: React.ComponentPropsWithRef<"thead">) => <thead {...props} />,
    // eslint-disable-next-line react/display-name
    TableBody: React.forwardRef<HTMLTableSectionElement, React.ComponentPropsWithRef<"tbody">>(
      (props, ref) => <tbody ref={ref} {...props} />
    ),
    TableRow: ({ "data-index": dataIndex, ...props }: React.ComponentPropsWithRef<"tr"> & { "data-index": number }) => {
      const s = displayed[dataIndex];
      return (
        <tr
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
        <div className={tableTitle}>{titleText}</div>
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
