"use client";

import { useState, useRef, useCallback, useEffect } from "react";
import type { LogEntry as ProtoLogEntry } from "@/gen/session/v1/session_pb";
import type { LogEntry } from "@/lib/hooks/useLogViewer";
import { formatRelativeTime, getUserTimezone, TIME_RANGE_PRESETS } from "@/lib/utils/datetime";
import { TimeRangePicker, type TimeRange } from "@/components/logs/TimeRangePicker";
import { FilterPill, FilterPills } from "@/components/logs/FilterPill";
import { ExportButton } from "@/components/logs/ExportButton";
import { PatternsView } from "@/components/logs/PatternsView";
import { LogViewer, type LogViewerHandle } from "@/components/shared/LogViewer";
import { ActionBar } from "@/components/ui/ActionBar";
import { usePageView } from "@/lib/analytics/usePageView";
import { useAnalytics } from "@/lib/analytics";
import * as styles from "./page.css";

type ViewMode = "table" | "patterns";

export default function LogsPage() {
  usePageView();
  const { track } = useAnalytics();

  // Time range and limit are the two real, wired filters for this page —
  // LogViewer owns search/level filtering itself (see LogViewerToolbar,
  // rendered inside LogViewer) since those need to apply to the same fetch
  // it's already doing; duplicating them here would just be a second,
  // easy-to-desync copy of the same state.
  const [timeRange, setTimeRange] = useState<TimeRange>(() => {
    const preset = TIME_RANGE_PRESETS.find((p) => p.value === "1h");
    const range = preset?.getRange() || { start: new Date(Date.now() - 60 * 60 * 1000), end: new Date() };
    return { ...range, preset: "1h" };
  });
  const [limit, setLimit] = useState(200);
  const [viewMode, setViewMode] = useState<ViewMode>("table");

  // Lifted from LogViewer via onStateChange — this is the single source of
  // truth for what's actually displayed, used by Export, the Patterns view,
  // and the footer count.
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [rawEntries, setRawEntries] = useState<ProtoLogEntry[]>([]);
  const [serverTotalCount, setServerTotalCount] = useState(0);
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null);

  const logViewerRef = useRef<LogViewerHandle>(null);

  const handleLogViewerStateChange = useCallback(
    (state: { logs: LogEntry[]; rawEntries: ProtoLogEntry[]; totalCount: number; lastRefresh: Date | null }) => {
      setLogs(state.logs);
      setRawEntries(state.rawEntries);
      setServerTotalCount(state.totalCount);
      setLastRefresh(state.lastRefresh);
    },
    [],
  );

  // "all" means no bound — everything else is a real time-bounded window.
  const activeTimeRange = timeRange.preset === "all" ? undefined : { start: timeRange.start, end: timeRange.end };

  const removeTimeRangeFilter = () => {
    const preset = TIME_RANGE_PRESETS.find((p) => p.value === "all");
    if (preset) {
      const range = preset.getRange();
      setTimeRange({ ...range, preset: "all" });
    }
  };

  const hasActiveFilters = Boolean(timeRange.preset && timeRange.preset !== "all");

  // LogViewer already implements /, Escape, g, G, =, ?, Cmd+F, and j/k
  // internally (see its own handleKeyDown) for everything scoped to the log
  // list itself — only page-level actions belong here.
  useEffect(() => {
    const isInputFocused = () => {
      const active = document.activeElement;
      return (
        active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement || active instanceof HTMLSelectElement
      );
    };
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "r" && !isInputFocused()) {
        e.preventDefault();
        logViewerRef.current?.refresh();
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, []);

  return (
    <main id="main-content" className={styles.container}>
      <header className={styles.header}>
        <h1>Application Logs</h1>
        <ActionBar scroll compact gap="md" className={styles.headerActions}>
          <TimeRangePicker value={timeRange} onChange={setTimeRange} />
          <span className={styles.timezone} title="Your local timezone">
            {getUserTimezone()}
          </span>
          <button
            onClick={() => {
              track({ name: "logs_refresh_clicked", category: "user_action" });
              logViewerRef.current?.refresh();
            }}
            className={styles.refreshButton}
            aria-label="Refresh logs"
            title={lastRefresh ? `Last updated: ${formatRelativeTime(lastRefresh.getTime())}` : "Not yet refreshed"}
          >
            🔄 Refresh
          </button>
          <ExportButton logs={rawEntries} disabled={rawEntries.length === 0} />
        </ActionBar>
      </header>

      <ActionBar scroll compact gap="md" className={styles.filters}>
        <div className={styles.filterGroup} role="tablist" aria-label="Log view">
          <button
            role="tab"
            aria-selected={viewMode === "table"}
            className={viewMode === "table" ? styles.viewTabActive : styles.viewTab}
            onClick={() => {
              track({ name: "logs_view_mode_changed", category: "user_action", labels: { mode: "table" } });
              setViewMode("table");
            }}
          >
            Table
          </button>
          <button
            role="tab"
            aria-selected={viewMode === "patterns"}
            className={viewMode === "patterns" ? styles.viewTabActive : styles.viewTab}
            onClick={() => {
              track({ name: "logs_view_mode_changed", category: "user_action", labels: { mode: "patterns" } });
              setViewMode("patterns");
            }}
          >
            Patterns
          </button>
        </div>

        <div className={styles.filterGroup}>
          <label htmlFor="limit">Limit:</label>
          <select
            id="limit"
            value={limit}
            onChange={(e) => setLimit(Number(e.target.value))}
            className={styles.select}
            aria-label="Results per page"
          >
            <option value="50">50</option>
            <option value="100">100</option>
            <option value="200">200</option>
            <option value="500">500</option>
            <option value="1000">1000</option>
            <option value="2000">2000</option>
          </select>
        </div>
      </ActionBar>

      {hasActiveFilters && (
        <FilterPills onClearAll={removeTimeRangeFilter}>
          <FilterPill
            label="Time"
            value={TIME_RANGE_PRESETS.find((p) => p.value === timeRange.preset)?.label || "Custom"}
            onRemove={removeTimeRangeFilter}
          />
        </FilterPills>
      )}

      {/* LogViewer stays mounted (and fetching) in both view modes so
          switching to Patterns and back doesn't lose live-tail state or
          re-fetch from scratch — only its visibility toggles. */}
      <div style={{ display: viewMode === "table" ? "contents" : "none" }}>
        <div className={styles.logsContainer}>
          <LogViewer
            ref={logViewerRef}
            source="app"
            timeRange={activeTimeRange}
            limit={limit}
            onStateChange={handleLogViewerStateChange}
          />
        </div>
      </div>
      {viewMode === "patterns" && (
        <div className={styles.logsContainer}>
          <PatternsView entries={logs} />
        </div>
      )}

      <footer className={styles.footer}>
        <span>
          Showing {logs.length} of {serverTotalCount} log entries
        </span>
        <span className={styles.shortcuts}>
          <kbd>R</kbd> Refresh • <kbd>/</kbd> Search • <kbd>?</kbd> All shortcuts
        </span>
      </footer>
    </main>
  );
}
