// +feature: insights-dashboard
"use client";

import React, { useState, useMemo, useCallback } from "react";
import { TableVirtuoso } from "react-virtuoso";
import Fuse from "fuse.js";
import type { SessionTokenSummary } from "@/gen/session/v1/insights_pb";
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
  empty,
  filterBar,
  searchInput,
  modelSelect,
  clearButton,
  virtualContainer,
  clickableRow,
} from "./SessionsTable.css";
import { fmtCost, fmtTokens, fmtPct, shortId } from "./insightsFormatters";

interface Props {
  sessions: SessionTokenSummary[];
  onSessionClick?: (session: SessionTokenSummary) => void;
}

function pathBasename(p: string): string {
  return p.split("/").pop() || p;
}

const VIRTUOSO_THRESHOLD = 50;

export function SessionsTable({ sessions, onSessionClick }: Props) {
  const [showOrphans, setShowOrphans] = useState(true);
  const [searchText, setSearchText] = useState("");
  const [modelFilter, setModelFilter] = useState("");

  const orphanCount = sessions.filter((s) => s.isOrphan).length;

  const fuse = useMemo(
    () => new Fuse(sessions, { keys: ["projectPath"], threshold: 0.4 }),
    [sessions]
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
      result = fuse.search(searchText).map((r) => r.item);
    } else {
      result = sessions;
    }

    if (modelFilter) {
      result = result.filter((s) => s.primaryModel === modelFilter);
    }

    if (!showOrphans) {
      result = result.filter((s) => !s.isOrphan);
    }

    return [...result].sort((a, b) => {
      const at = a.lastMessageAt ? Number(a.lastMessageAt.seconds) : 0;
      const bt = b.lastMessageAt ? Number(b.lastMessageAt.seconds) : 0;
      return bt - at;
    });
  }, [sessions, searchText, modelFilter, showOrphans, fuse]);

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

  const headerContent = () => (
    <tr>
      <th className={th}>Session</th>
      <th className={th}>Model</th>
      <th className={th}>Path</th>
      <th className={thRight}>Input</th>
      <th className={thRight}>Output</th>
      <th className={thRight}>Cache</th>
      <th className={thRight}>Cost</th>
    </tr>
  );

  const renderCells = (_index: number, s: SessionTokenSummary) => (
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
      </td>
      <td className={td} title={s.primaryModel}>{s.primaryModel || "—"}</td>
      <td className={td} title={s.projectPath}>{pathBasename(s.projectPath) || "—"}</td>
      <td className={tdRight}>{fmtTokens(s.totalInputTokens)}</td>
      <td className={tdRight}>{fmtTokens(s.totalOutputTokens)}</td>
      <td className={tdRight}>{fmtPct(s.cacheHitRate)}</td>
      <td className={tdRight}>{fmtCost(s.estimatedCostUsd)}</td>
    </>
  );

  const virtuosoComponents = useMemo(() => ({
    Table: ({ style: s, ...props }: React.ComponentPropsWithRef<"table">) => (
      <table className={table} style={s} {...props} />
    ),
    TableHead: (props: React.ComponentPropsWithRef<"thead">) => <thead {...props} />,
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
