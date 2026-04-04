"use client";

import { useEffect, useState, useRef, useCallback } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { LogEntry } from "@/gen/session/v1/session_pb";
import { timestampFromDate, timestampDate } from "@bufbuild/protobuf/wkt";
import { formatTimestamp, formatRelativeTime, getUserTimezone, TIME_RANGE_PRESETS } from "@/lib/utils/datetime";
import { getApiBaseUrl } from "@/lib/config";
import { useDebounce } from "@/lib/hooks/useDebounce";
import { TimeRangePicker, type TimeRange } from "@/components/logs/TimeRangePicker";
import { FilterPill, FilterPills } from "@/components/logs/FilterPill";
import { MultiSelect, LOG_LEVEL_OPTIONS } from "@/components/logs/MultiSelect";
import { LiveTailToggle } from "@/components/logs/LiveTailToggle";
import { ExportButton } from "@/components/logs/ExportButton";
import { SearchWithHistory } from "@/components/logs/SearchWithHistory";
import { DensityToggle, type LogDensity } from "@/components/logs/DensityToggle";
import { useLiveTail } from "@/lib/hooks/useLiveTail";
import styles from "./page.module.css";

export default function LogsPage() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Filter states
  const [searchQuery, setSearchQuery] = useState("");
  const [levelFilters, setLevelFilters] = useState<string[]>([]);
  const [limit, setLimit] = useState(100);
  const [timeRange, setTimeRange] = useState<TimeRange>(() => {
    const preset = TIME_RANGE_PRESETS.find(p => p.value === '1h');
    const range = preset?.getRange() || { start: new Date(Date.now() - 60 * 60 * 1000), end: new Date() };
    return { ...range, preset: '1h' };
  });

  // UI states
  const [expandedRow, setExpandedRow] = useState<number | null>(null);
  const [lastRefresh, setLastRefresh] = useState<Date>(new Date());
  const [density, setDensity] = useState<LogDensity>('comfortable');

  // Pagination states
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [totalCount, setTotalCount] = useState(0);

  // Live tail states
  const [liveTailEnabled, setLiveTailEnabled] = useState(false);
  const [liveTailInterval, setLiveTailInterval] = useState(2000);

  // Debounced search query
  const debouncedSearchQuery = useDebounce(searchQuery, 300);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const logsContainerRef = useRef<HTMLDivElement>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  // Initialize ConnectRPC client
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
    });

    clientRef.current = createClient(SessionService, transport);
  }, []);

  // Fetch logs from API
  const fetchLogs = useCallback(async (resetOffset = true) => {
    if (!clientRef.current) return;

    // Cancel previous request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    abortControllerRef.current = new AbortController();

    if (resetOffset) {
      setLoading(true);
    }
    setError(null);

    const newOffset = resetOffset ? 0 : offset;

    try {
      // Build level filter - if multiple levels selected, we'll filter client-side for now
      // (backend supports single level; for multi-level we'd need API update)
      const singleLevelFilter = levelFilters.length === 1 ? levelFilters[0] : undefined;

      const response = await clientRef.current.getLogs({
        searchQuery: debouncedSearchQuery || undefined,
        level: singleLevelFilter,
        limit: limit,
        offset: newOffset,
        startTime: timestampFromDate(timeRange.start),
        endTime: timestampFromDate(timeRange.end),
      });

      let entries = response.entries || [];

      // Client-side multi-level filtering if needed
      if (levelFilters.length > 1) {
        entries = entries.filter(entry =>
          levelFilters.includes(entry.level.toUpperCase())
        );
      }

      if (resetOffset) {
        setLogs(entries);
        setOffset(entries.length);
      } else {
        setLogs((prev) => [...prev, ...entries]);
        setOffset((prev) => prev + entries.length);
      }

      setHasMore(response.hasMore || false);
      setTotalCount(response.totalCount || 0);
      setLastRefresh(new Date());
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') {
        return; // Ignore aborted requests
      }
      setError(err instanceof Error ? err.message : "Failed to fetch logs");
      console.error("Failed to fetch logs:", err);
    } finally {
      setLoading(false);
    }
  }, [debouncedSearchQuery, levelFilters, limit, offset, timeRange]);

  // Load more logs when scrolling to bottom
  const loadMoreLogs = useCallback(async () => {
    if (!clientRef.current || loadingMore || !hasMore) return;

    setLoadingMore(true);

    try {
      const singleLevelFilter = levelFilters.length === 1 ? levelFilters[0] : undefined;

      const response = await clientRef.current.getLogs({
        searchQuery: debouncedSearchQuery || undefined,
        level: singleLevelFilter,
        limit: limit,
        offset: offset,
        startTime: timestampFromDate(timeRange.start),
        endTime: timestampFromDate(timeRange.end),
      });

      let entries = response.entries || [];
      if (levelFilters.length > 1) {
        entries = entries.filter(entry =>
          levelFilters.includes(entry.level.toUpperCase())
        );
      }

      setLogs((prev) => [...prev, ...entries]);
      setOffset((prev) => prev + entries.length);
      setHasMore(response.hasMore || false);
      setTotalCount(response.totalCount || 0);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load more logs");
      console.error("Failed to load more logs:", err);
    } finally {
      setLoadingMore(false);
    }
  }, [clientRef, loadingMore, hasMore, debouncedSearchQuery, levelFilters, limit, offset, timeRange]);

  // Fetch logs on mount and when filters change
  useEffect(() => {
    fetchLogs(true);
  }, [debouncedSearchQuery, levelFilters, limit, timeRange]);

  // Infinite scroll
  useEffect(() => {
    const container = logsContainerRef.current;
    if (!container) return;

    const handleScroll = () => {
      const { scrollTop, scrollHeight, clientHeight } = container;
      if (scrollHeight - scrollTop - clientHeight < 100 && hasMore && !loadingMore) {
        loadMoreLogs();
      }
    };

    container.addEventListener("scroll", handleScroll);
    return () => container.removeEventListener("scroll", handleScroll);
  }, [hasMore, loadingMore, loadMoreLogs]);

  // Live tail hook - for auto-refreshing logs
  const liveTailFetch = useCallback(async () => {
    // When live tailing, update time range end to now and refresh
    if (liveTailEnabled) {
      // For live tail, always use "now" as end time
      setTimeRange(prev => ({
        ...prev,
        end: new Date(),
      }));
    }
    await fetchLogs(true);
  }, [liveTailEnabled, fetchLogs]);

  const [liveTailState, liveTailControls] = useLiveTail(liveTailFetch, {
    interval: liveTailInterval,
    enabled: liveTailEnabled,
  });

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Cmd/Ctrl + K to focus search
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        document.getElementById('search')?.focus();
      }
      // R to refresh (when not in input)
      if (e.key === 'r' && !isInputFocused()) {
        e.preventDefault();
        fetchLogs(true);
      }
      // L to toggle live tail (when not in input)
      if (e.key === 'l' && !isInputFocused()) {
        e.preventDefault();
        setLiveTailEnabled(prev => !prev);
      }
      // Space to pause/resume live tail when active (when not in input)
      if (e.key === ' ' && liveTailEnabled && !isInputFocused()) {
        e.preventDefault();
        liveTailControls.toggle();
      }
      // Escape to clear search
      if (e.key === 'Escape') {
        setSearchQuery('');
        setExpandedRow(null);
      }
    };

    const isInputFocused = () => {
      const activeElement = document.activeElement;
      return activeElement instanceof HTMLInputElement ||
             activeElement instanceof HTMLTextAreaElement ||
             activeElement instanceof HTMLSelectElement;
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [fetchLogs, liveTailEnabled, liveTailControls]);

  // Get CSS class for log level
  const getLevelClass = (level: string) => {
    switch (level.toUpperCase()) {
      case "DEBUG": return styles.levelDebug;
      case "INFO": return styles.levelInfo;
      case "WARNING":
      case "WARN": return styles.levelWarning;
      case "ERROR": return styles.levelError;
      case "FATAL": return styles.levelFatal;
      default: return "";
    }
  };

  // Get level color for filter pills
  const getLevelColor = (level: string) => {
    switch (level.toUpperCase()) {
      case "DEBUG": return "#6c757d";
      case "INFO": return "#17a2b8";
      case "WARNING": return "#ffc107";
      case "ERROR": return "#dc3545";
      case "FATAL": return "#ff0000";
      default: return undefined;
    }
  };

  // Click-to-filter handlers
  const handleLevelClick = (level: string) => {
    const upperLevel = level.toUpperCase();
    if (!levelFilters.includes(upperLevel)) {
      setLevelFilters([...levelFilters, upperLevel]);
    }
  };

  const handleSourceClick = (source: string) => {
    setSearchQuery(prev => {
      const sourceFilter = `source:${source}`;
      if (prev.includes(sourceFilter)) return prev;
      return prev ? `${prev} ${sourceFilter}` : sourceFilter;
    });
  };

  // Remove filter handlers
  const removeSearchFilter = () => setSearchQuery('');
  const removeLevelFilter = (level: string) => {
    setLevelFilters(prev => prev.filter(l => l !== level));
  };
  const removeTimeRangeFilter = () => {
    const preset = TIME_RANGE_PRESETS.find(p => p.value === 'all');
    if (preset) {
      const range = preset.getRange();
      setTimeRange({ ...range, preset: 'all' });
    }
  };
  const clearAllFilters = () => {
    setSearchQuery('');
    setLevelFilters([]);
    const preset = TIME_RANGE_PRESETS.find(p => p.value === '1h');
    if (preset) {
      const range = preset.getRange();
      setTimeRange({ ...range, preset: '1h' });
    }
  };

  // Check if any filters are active
  const hasActiveFilters = searchQuery || levelFilters.length > 0 || (timeRange.preset && timeRange.preset !== 'all');

  // Toggle row expansion
  const toggleRowExpand = (index: number) => {
    setExpandedRow(prev => prev === index ? null : index);
  };

  // Copy log to clipboard
  const copyLog = async (log: LogEntry) => {
    const text = `[${log.timestamp ? timestampDate(log.timestamp).toISOString() : 'N/A'}] [${log.level}] [${log.source}] ${log.message}`;
    await navigator.clipboard.writeText(text);
  };

  return (
    <main id="main-content" className={styles.container}>
      <header className={styles.header}>
        <h1>Application Logs</h1>
        <div className={styles.headerActions}>
          <LiveTailToggle
            isEnabled={liveTailEnabled}
            onToggle={() => setLiveTailEnabled(prev => !prev)}
            isPaused={liveTailState.isPaused}
            onPauseToggle={liveTailControls.toggle}
            interval={liveTailInterval}
            onIntervalChange={setLiveTailInterval}
            lastRefresh={liveTailState.lastFetch}
          />
          <TimeRangePicker
            value={timeRange}
            onChange={setTimeRange}
          />
          <span className={styles.timezone} title="Your local timezone">
            {getUserTimezone()}
          </span>
          <button
            onClick={() => fetchLogs(true)}
            className={styles.refreshButton}
            aria-label="Refresh logs"
            title={`Last updated: ${formatRelativeTime(lastRefresh.getTime())}`}
          >
            🔄 Refresh
          </button>
          <ExportButton logs={logs} disabled={loading} />
        </div>
      </header>

      <div className={styles.filters}>
        <div className={styles.filterGroup}>
          <label htmlFor="search">Search:</label>
          <SearchWithHistory
            id="search"
            value={searchQuery}
            onChange={setSearchQuery}
            placeholder="Search logs... (Cmd+K)"
            className={styles.searchHistoryWrapper}
          />
        </div>

        <MultiSelect
          label="Level"
          options={LOG_LEVEL_OPTIONS}
          value={levelFilters}
          onChange={setLevelFilters}
          placeholder="All"
        />

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
          </select>
        </div>

        <div className={styles.filterGroup}>
          <label>Density:</label>
          <DensityToggle value={density} onChange={setDensity} />
        </div>
      </div>

      {/* Active Filter Pills */}
      {hasActiveFilters && (
        <FilterPills onClearAll={clearAllFilters}>
          {searchQuery && (
            <FilterPill
              label="Search"
              value={searchQuery}
              onRemove={removeSearchFilter}
            />
          )}
          {levelFilters.map(level => (
            <FilterPill
              key={level}
              label="Level"
              value={level}
              color={getLevelColor(level)}
              onRemove={() => removeLevelFilter(level)}
            />
          ))}
          {timeRange.preset && timeRange.preset !== 'all' && (
            <FilterPill
              label="Time"
              value={TIME_RANGE_PRESETS.find(p => p.value === timeRange.preset)?.label || 'Custom'}
              onRemove={removeTimeRangeFilter}
            />
          )}
        </FilterPills>
      )}

      {loading && (
        <div className={styles.loading} role="status" aria-live="polite">
          Loading logs...
        </div>
      )}

      {error && (
        <div className={styles.error} role="alert">
          Error: {error}
        </div>
      )}

      {!loading && !error && logs.length === 0 && (
        <div className={styles.noLogs}>
          <h3>No logs found</h3>
          <p>Try:</p>
          <ul>
            <li>Expanding your time range</li>
            <li>Removing some filters</li>
            <li>Checking your search query</li>
          </ul>
        </div>
      )}

      {!loading && !error && logs.length > 0 && (
        <div
          className={styles.logsContainer}
          ref={logsContainerRef}
          role="log"
          aria-label="Log entries"
          aria-live="polite"
        >
          <table className={`${styles.logsTable} ${styles[`density${density.charAt(0).toUpperCase()}${density.slice(1)}`]}`}>
            <thead>
              <tr>
                <th className={styles.expandColumn} aria-label="Expand"></th>
                <th className={styles.timestampColumn}>Timestamp</th>
                <th className={styles.levelColumn}>Level</th>
                <th className={styles.sourceColumn}>Source</th>
                <th className={styles.messageColumn}>Message</th>
                <th className={styles.actionsColumn} aria-label="Actions"></th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log, index) => (
                <>
                  <tr
                    key={index}
                    className={`${styles.logRow} ${expandedRow === index ? styles.logRowExpanded : ''}`}
                  >
                    <td className={styles.expandCell}>
                      <button
                        className={styles.expandButton}
                        onClick={() => toggleRowExpand(index)}
                        aria-expanded={expandedRow === index}
                        aria-label={expandedRow === index ? 'Collapse log details' : 'Expand log details'}
                      >
                        {expandedRow === index ? '▼' : '▶'}
                      </button>
                    </td>
                    <td
                      className={styles.timestamp}
                      title={log.timestamp ? formatTimestamp(timestampDate(log.timestamp)) : 'N/A'}
                    >
                      {log.timestamp
                        ? formatRelativeTime(timestampDate(log.timestamp).getTime())
                        : "N/A"}
                    </td>
                    <td
                      className={`${styles.level} ${getLevelClass(log.level)} ${styles.clickable}`}
                      onClick={() => handleLevelClick(log.level)}
                      title="Click to filter by this level"
                      role="button"
                      tabIndex={0}
                      onKeyDown={(e) => e.key === 'Enter' && handleLevelClick(log.level)}
                    >
                      {log.level}
                      <span className={styles.filterIcon}>⊕</span>
                    </td>
                    <td
                      className={`${styles.source} ${styles.clickable}`}
                      onClick={() => log.source && handleSourceClick(log.source)}
                      title="Click to filter by this source"
                      role="button"
                      tabIndex={0}
                      onKeyDown={(e) => e.key === 'Enter' && log.source && handleSourceClick(log.source)}
                    >
                      {log.source || "-"}
                      {log.source && <span className={styles.filterIcon}>⊕</span>}
                    </td>
                    <td className={styles.message}>
                      {log.message.length > 150 && expandedRow !== index
                        ? `${log.message.substring(0, 150)}...`
                        : log.message}
                    </td>
                    <td className={styles.actionsCell}>
                      <button
                        className={styles.actionButton}
                        onClick={() => copyLog(log)}
                        title="Copy log entry"
                        aria-label="Copy log entry to clipboard"
                      >
                        📋
                      </button>
                    </td>
                  </tr>
                  {expandedRow === index && (
                    <tr className={styles.expandedRow}>
                      <td colSpan={6}>
                        <div className={styles.logDetail}>
                          <div className={styles.logDetailSection}>
                            <strong>Full Timestamp:</strong>
                            <span>{log.timestamp ? formatTimestamp(timestampDate(log.timestamp)) : 'N/A'}</span>
                          </div>
                          <div className={styles.logDetailSection}>
                            <strong>Level:</strong>
                            <span className={getLevelClass(log.level)}>{log.level}</span>
                          </div>
                          <div className={styles.logDetailSection}>
                            <strong>Source:</strong>
                            <span>{log.source || '-'}</span>
                          </div>
                          <div className={styles.logDetailSection}>
                            <strong>Message:</strong>
                            <pre className={styles.logDetailMessage}>{log.message}</pre>
                          </div>
                        </div>
                      </td>
                    </tr>
                  )}
                </>
              ))}
            </tbody>
          </table>

          {loadingMore && (
            <div className={styles.loadingMore} role="status" aria-live="polite">
              Loading more logs...
            </div>
          )}

          {!hasMore && logs.length > 0 && (
            <div className={styles.endOfLogs}>
              End of logs (showing all {totalCount} entries)
            </div>
          )}
        </div>
      )}

      <footer className={styles.footer}>
        <span>
          Showing {logs.length} of {totalCount} log entries
          {hasMore && " (scroll for more)"}
          {liveTailEnabled && (
            <span className={styles.liveTailStatus}>
              {liveTailState.isPaused ? ' • Live tail paused' : ` • Live tail (${liveTailInterval / 1000}s)`}
            </span>
          )}
        </span>
        <span className={styles.shortcuts}>
          <kbd>⌘K</kbd> Search • <kbd>R</kbd> Refresh • <kbd>L</kbd> Live Tail • <kbd>Esc</kbd> Clear
        </span>
      </footer>
    </main>
  );
}
