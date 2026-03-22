"use client";

import { useEffect, useState, useRef, useCallback } from "react";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";
import { formatTimestamp, formatRelativeTime } from "@/lib/utils/datetime";
import { useDebounce } from "@/lib/hooks/useDebounce";
import { MultiSelect, LOG_LEVEL_OPTIONS } from "@/components/logs/MultiSelect";
import { LiveTailToggle } from "@/components/logs/LiveTailToggle";
import { useLiveTail } from "@/lib/hooks/useLiveTail";
import styles from "./SessionLogsTab.module.css";

interface LogEntry {
  timestamp?: { toDate(): Date };
  level: string;
  message: string;
  source?: string;
}

interface SessionLogsTabProps {
  sessionId: string;
  baseUrl: string;
}

export function SessionLogsTab({ sessionId, baseUrl }: SessionLogsTabProps) {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [searchQuery, setSearchQuery] = useState("");
  const [levelFilters, setLevelFilters] = useState<string[]>([]);
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [totalCount, setTotalCount] = useState(0);
  const [liveTailEnabled, setLiveTailEnabled] = useState(false);
  const [liveTailInterval, setLiveTailInterval] = useState(3000);

  const debouncedSearch = useDebounce(searchQuery, 300);
  const clientRef = useRef<ReturnType<typeof createPromiseClient<typeof SessionService>> | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    clientRef.current = createPromiseClient(
      SessionService,
      createConnectTransport({ baseUrl })
    );
  }, [baseUrl]);

  const fetchLogs = useCallback(async (reset = true) => {
    if (!clientRef.current) return;
    if (abortRef.current) abortRef.current.abort();
    abortRef.current = new AbortController();

    if (reset) setLoading(true);
    setError(null);

    const currentOffset = reset ? 0 : offset;
    try {
      const singleLevel = levelFilters.length === 1 ? levelFilters[0] : undefined;
      const response = await clientRef.current.getLogs({
        sessionId,
        searchQuery: debouncedSearch || undefined,
        level: singleLevel,
        limit: 100,
        offset: currentOffset,
      });

      let entries = response.entries || [];
      if (levelFilters.length > 1) {
        entries = entries.filter(e => levelFilters.includes(e.level.toUpperCase()));
      }

      if (reset) {
        setLogs(entries);
        setOffset(entries.length);
      } else {
        setLogs(prev => [...prev, ...entries]);
        setOffset(prev => prev + entries.length);
      }
      setHasMore(response.hasMore || false);
      setTotalCount(response.totalCount || 0);
    } catch (err) {
      if (err instanceof Error && err.name === "AbortError") return;
      setError(err instanceof Error ? err.message : "Failed to fetch logs");
    } finally {
      setLoading(false);
    }
  }, [sessionId, debouncedSearch, levelFilters, offset]);

  useEffect(() => {
    fetchLogs(true);
  }, [sessionId, debouncedSearch, levelFilters]);

  const [liveTailState, liveTailControls] = useLiveTail(
    useCallback(() => fetchLogs(true), [fetchLogs]),
    { interval: liveTailInterval, enabled: liveTailEnabled }
  );

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

  return (
    <div className={styles.container}>
      <div className={styles.toolbar}>
        <input
          type="text"
          className={styles.searchInput}
          placeholder="Search logs..."
          value={searchQuery}
          onChange={e => setSearchQuery(e.target.value)}
          aria-label="Search logs"
        />
        <MultiSelect
          label="Level"
          options={LOG_LEVEL_OPTIONS}
          value={levelFilters}
          onChange={setLevelFilters}
          placeholder="All"
        />
        <LiveTailToggle
          isEnabled={liveTailEnabled}
          onToggle={() => setLiveTailEnabled(prev => !prev)}
          isPaused={liveTailState.isPaused}
          onPauseToggle={liveTailControls.toggle}
          interval={liveTailInterval}
          onIntervalChange={setLiveTailInterval}
          lastRefresh={liveTailState.lastFetch}
        />
        <button
          className={styles.refreshButton}
          onClick={() => fetchLogs(true)}
          aria-label="Refresh logs"
        >
          Refresh
        </button>
      </div>

      {loading && (
        <div className={styles.status} role="status">Loading logs...</div>
      )}

      {error && (
        <div className={styles.statusError} role="alert">Error: {error}</div>
      )}

      {!loading && !error && logs.length === 0 && (
        <div className={styles.empty}>
          No logs recorded for this session yet.
        </div>
      )}

      {!loading && !error && logs.length > 0 && (
        <>
          <div className={styles.tableWrapper}>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th className={styles.colTimestamp}>Timestamp</th>
                  <th className={styles.colLevel}>Level</th>
                  <th className={styles.colSource}>Source</th>
                  <th className={styles.colMessage}>Message</th>
                </tr>
              </thead>
              <tbody>
                {logs.map((entry, i) => (
                  <tr key={i} className={styles.row}>
                    <td
                      className={styles.timestamp}
                      title={entry.timestamp ? formatTimestamp(entry.timestamp.toDate()) : "N/A"}
                    >
                      {entry.timestamp
                        ? formatRelativeTime(entry.timestamp.toDate().getTime())
                        : "N/A"}
                    </td>
                    <td className={`${styles.level} ${getLevelClass(entry.level)}`}>
                      {entry.level}
                    </td>
                    <td className={styles.source}>{entry.source || "-"}</td>
                    <td className={styles.message}>{entry.message}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {hasMore && (
            <button
              className={styles.loadMoreButton}
              onClick={() => fetchLogs(false)}
              disabled={loadingMore}
            >
              {loadingMore ? "Loading..." : `Load more (${totalCount - logs.length} remaining)`}
            </button>
          )}
        </>
      )}
    </div>
  );
}
