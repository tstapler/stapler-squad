"use client";

import { useEffect, useState, useRef } from "react";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";
import { formatTimestamp, getUserTimezone } from "@/lib/utils/datetime";
import styles from "./page.module.css";

interface LogEntry {
  timestamp?: {
    toDate(): Date;
  };
  level: string;
  message: string;
  source?: string;
}

export default function LogsPage() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Filter states
  const [searchQuery, setSearchQuery] = useState("");
  const [levelFilter, setLevelFilter] = useState<string>("");
  const [limit, setLimit] = useState(100);

  // Pagination states
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [totalCount, setTotalCount] = useState(0);

  const clientRef = useRef<any>(null);
  const logsContainerRef = useRef<HTMLDivElement>(null);

  // Initialize ConnectRPC client
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: window.location.origin,
    });

    clientRef.current = createPromiseClient(SessionService, transport);
  }, []);

  // Fetch logs from API (initial load or filter change)
  const fetchLogs = async (resetOffset = true) => {
    if (!clientRef.current) return;

    setLoading(true);
    setError(null);

    const newOffset = resetOffset ? 0 : offset;

    try {
      const response = await clientRef.current.getLogs({
        searchQuery: searchQuery || undefined,
        level: levelFilter || undefined,
        limit: limit,
        offset: newOffset,
      });

      if (resetOffset) {
        setLogs(response.entries || []);
        setOffset(response.entries?.length || 0);
      } else {
        setLogs((prev) => [...prev, ...(response.entries || [])]);
        setOffset((prev) => prev + (response.entries?.length || 0));
      }

      setHasMore(response.hasMore || false);
      setTotalCount(response.totalCount || 0);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch logs");
      console.error("Failed to fetch logs:", err);
    } finally {
      setLoading(false);
    }
  };

  // Load more logs when scrolling to bottom
  const loadMoreLogs = async () => {
    if (!clientRef.current || loadingMore || !hasMore) return;

    setLoadingMore(true);

    try {
      const response = await clientRef.current.getLogs({
        searchQuery: searchQuery || undefined,
        level: levelFilter || undefined,
        limit: limit,
        offset: offset,
      });

      setLogs((prev) => [...prev, ...(response.entries || [])]);
      setOffset((prev) => prev + (response.entries?.length || 0));
      setHasMore(response.hasMore || false);
      setTotalCount(response.totalCount || 0);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load more logs");
      console.error("Failed to load more logs:", err);
    } finally {
      setLoadingMore(false);
    }
  };

  // Fetch logs on mount and when filters change
  useEffect(() => {
    fetchLogs(true);
  }, [levelFilter, limit]);

  // Infinite scroll: load more when scrolling to bottom
  useEffect(() => {
    const container = logsContainerRef.current;
    if (!container) return;

    const handleScroll = () => {
      const { scrollTop, scrollHeight, clientHeight } = container;
      // Load more when within 100px of bottom
      if (scrollHeight - scrollTop - clientHeight < 100 && hasMore && !loadingMore) {
        loadMoreLogs();
      }
    };

    container.addEventListener("scroll", handleScroll);
    return () => container.removeEventListener("scroll", handleScroll);
  }, [hasMore, loadingMore, offset]);

  // Manual search (triggered by button)
  const handleSearch = () => {
    fetchLogs(true);
  };

  // Get CSS class for log level
  const getLevelClass = (level: string) => {
    switch (level.toUpperCase()) {
      case "DEBUG":
        return styles.levelDebug;
      case "INFO":
        return styles.levelInfo;
      case "WARNING":
      case "WARN":
        return styles.levelWarning;
      case "ERROR":
        return styles.levelError;
      case "FATAL":
        return styles.levelFatal;
      default:
        return "";
    }
  };

  return (
    <div className={styles.container}>
      <header className={styles.header}>
        <h1>Application Logs</h1>
        <div className={styles.headerActions}>
          <span className={styles.timezone} title="Your local timezone">
            🕐 {getUserTimezone()}
          </span>
          <button onClick={() => fetchLogs(true)} className={styles.refreshButton}>
            🔄 Refresh
          </button>
        </div>
      </header>

      <div className={styles.filters}>
        <div className={styles.filterGroup}>
          <label htmlFor="search">Search:</label>
          <input
            id="search"
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleSearch()}
            placeholder="Search logs..."
            className={styles.searchInput}
          />
          <button onClick={handleSearch} className={styles.searchButton}>
            Search
          </button>
        </div>

        <div className={styles.filterGroup}>
          <label htmlFor="level">Level:</label>
          <select
            id="level"
            value={levelFilter}
            onChange={(e) => setLevelFilter(e.target.value)}
            className={styles.select}
          >
            <option value="">All</option>
            <option value="DEBUG">DEBUG</option>
            <option value="INFO">INFO</option>
            <option value="WARNING">WARNING</option>
            <option value="ERROR">ERROR</option>
            <option value="FATAL">FATAL</option>
          </select>
        </div>

        <div className={styles.filterGroup}>
          <label htmlFor="limit">Limit:</label>
          <select
            id="limit"
            value={limit}
            onChange={(e) => setLimit(Number(e.target.value))}
            className={styles.select}
          >
            <option value="50">50</option>
            <option value="100">100</option>
            <option value="200">200</option>
            <option value="500">500</option>
            <option value="1000">1000</option>
          </select>
        </div>
      </div>

      {loading && <div className={styles.loading}>Loading logs...</div>}

      {error && <div className={styles.error}>Error: {error}</div>}

      {!loading && !error && logs.length === 0 && (
        <div className={styles.noLogs}>No logs found matching your filters.</div>
      )}

      {!loading && !error && logs.length > 0 && (
        <div className={styles.logsContainer} ref={logsContainerRef}>
          <table className={styles.logsTable}>
            <thead>
              <tr>
                <th className={styles.timestampColumn}>Timestamp</th>
                <th className={styles.levelColumn}>Level</th>
                <th className={styles.sourceColumn}>Source</th>
                <th className={styles.messageColumn}>Message</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log, index) => (
                <tr key={index} className={styles.logRow}>
                  <td className={styles.timestamp}>
                    {log.timestamp
                      ? formatTimestamp(log.timestamp.toDate())
                      : "N/A"}
                  </td>
                  <td className={`${styles.level} ${getLevelClass(log.level)}`}>
                    {log.level}
                  </td>
                  <td className={styles.source}>{log.source || "-"}</td>
                  <td className={styles.message}>{log.message}</td>
                </tr>
              ))}
            </tbody>
          </table>

          {loadingMore && (
            <div className={styles.loadingMore}>Loading more logs...</div>
          )}

          {!hasMore && logs.length > 0 && (
            <div className={styles.endOfLogs}>
              End of logs (showing all {totalCount} entries)
            </div>
          )}
        </div>
      )}

      <footer className={styles.footer}>
        Showing {logs.length} of {totalCount} log entries
        {hasMore && " (scroll for more)"}
      </footer>
    </div>
  );
}
