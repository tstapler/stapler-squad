"use client";

import { useState, useEffect, useCallback } from "react";
import { getApiBaseUrl } from "@/lib/config";
import styles from "./page.module.css";

interface EscapeCodeEntry {
  code: string;
  humanReadable: string;
  category: string;
  count: number;
  firstSeen: string;
  lastSeen: string;
  sessionIds: string[];
}

interface EscapeCodeStats {
  enabled: boolean;
  totalCodes: number;
  uniqueCodes: number;
  categoryCounts: Record<string, number>;
  topCodes: EscapeCodeEntry[];
  recentCodes: EscapeCodeEntry[];
}

export default function EscapeCodesPage() {
  const [entries, setEntries] = useState<EscapeCodeEntry[]>([]);
  const [stats, setStats] = useState<EscapeCodeStats | null>(null);
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [categoryFilter, setCategoryFilter] = useState<string>("all");
  const [sortBy, setSortBy] = useState<"count" | "recent">("count");

  const baseUrl = getApiBaseUrl();

  const fetchStatus = useCallback(async () => {
    try {
      const response = await fetch(`${baseUrl}/debug/escape-codes/status`);
      const data = await response.json();
      setEnabled(data.enabled);
    } catch (err) {
      console.error("Failed to fetch status:", err);
    }
  }, [baseUrl]);

  const fetchData = useCallback(async () => {
    try {
      setLoading(true);
      const [entriesRes, statsRes] = await Promise.all([
        fetch(`${baseUrl}/debug/escape-codes`),
        fetch(`${baseUrl}/debug/escape-codes/stats`),
      ]);

      if (!entriesRes.ok || !statsRes.ok) {
        throw new Error("Failed to fetch data");
      }

      const entriesData = await entriesRes.json();
      const statsData = await statsRes.json();

      setEntries(entriesData || []);
      setStats(statsData);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
    } finally {
      setLoading(false);
    }
  }, [baseUrl]);

  const toggleTracking = async () => {
    try {
      const response = await fetch(`${baseUrl}/debug/escape-codes/toggle`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enabled: !enabled }),
      });
      const data = await response.json();
      setEnabled(data.enabled);
    } catch (err) {
      console.error("Failed to toggle tracking:", err);
    }
  };

  const clearData = async () => {
    if (!confirm("Are you sure you want to clear all escape code data?")) {
      return;
    }
    try {
      await fetch(`${baseUrl}/debug/escape-codes`, { method: "DELETE" });
      fetchData();
    } catch (err) {
      console.error("Failed to clear data:", err);
    }
  };

  const exportData = () => {
    window.open(`${baseUrl}/debug/escape-codes/export`, "_blank");
  };

  useEffect(() => {
    fetchStatus();
    fetchData();

    // Poll for updates every 5 seconds when tracking is enabled
    const interval = setInterval(() => {
      if (enabled) {
        fetchData();
      }
    }, 5000);

    return () => clearInterval(interval);
  }, [enabled, fetchStatus, fetchData]);

  // Filter and sort entries
  const filteredEntries = entries
    .filter((e) => categoryFilter === "all" || e.category === categoryFilter)
    .sort((a, b) => {
      if (sortBy === "count") {
        return b.count - a.count;
      }
      return new Date(b.lastSeen).getTime() - new Date(a.lastSeen).getTime();
    });

  // Get unique categories
  const categories = Array.from(new Set(entries.map((e) => e.category))).sort();

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  const formatHex = (hex: string) => {
    // Group into pairs for readability
    return hex.match(/.{1,2}/g)?.join(" ") || hex;
  };

  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr);
    return date.toLocaleTimeString();
  };

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <h1>Terminal Escape Code Analytics</h1>
        <p className={styles.subtitle}>
          Debug terminal rendering issues by tracking escape sequences
        </p>
      </header>

      <div className={styles.controls}>
        <button
          className={`${styles.button} ${enabled ? styles.buttonActive : ""}`}
          onClick={toggleTracking}
        >
          {enabled ? "Tracking: ON" : "Tracking: OFF"}
        </button>
        <button className={styles.button} onClick={fetchData}>
          Refresh
        </button>
        <button className={styles.button} onClick={exportData}>
          Export JSON
        </button>
        <button className={`${styles.button} ${styles.buttonDanger}`} onClick={clearData}>
          Clear Data
        </button>
      </div>

      {error && <div className={styles.error}>Error: {error}</div>}

      {stats && (
        <div className={styles.statsGrid}>
          <div className={styles.statCard}>
            <div className={styles.statValue}>{stats.totalCodes}</div>
            <div className={styles.statLabel}>Total Codes</div>
          </div>
          <div className={styles.statCard}>
            <div className={styles.statValue}>{stats.uniqueCodes}</div>
            <div className={styles.statLabel}>Unique Codes</div>
          </div>
          {Object.entries(stats.categoryCounts || {}).map(([category, count]) => (
            <div key={category} className={styles.statCard}>
              <div className={styles.statValue}>{count}</div>
              <div className={styles.statLabel}>{category}</div>
            </div>
          ))}
        </div>
      )}

      <div className={styles.filters}>
        <label>
          Category:
          <select
            value={categoryFilter}
            onChange={(e) => setCategoryFilter(e.target.value)}
            className={styles.select}
          >
            <option value="all">All Categories</option>
            {categories.map((cat) => (
              <option key={cat} value={cat}>
                {cat}
              </option>
            ))}
          </select>
        </label>
        <label>
          Sort by:
          <select
            value={sortBy}
            onChange={(e) => setSortBy(e.target.value as "count" | "recent")}
            className={styles.select}
          >
            <option value="count">Count (High to Low)</option>
            <option value="recent">Most Recent</option>
          </select>
        </label>
      </div>

      {loading ? (
        <div className={styles.loading}>Loading...</div>
      ) : (
        <div className={styles.tableContainer}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th>Hex Code</th>
                <th>Description</th>
                <th>Category</th>
                <th>Count</th>
                <th>Last Seen</th>
                <th>Sessions</th>
              </tr>
            </thead>
            <tbody>
              {filteredEntries.map((entry) => (
                <tr key={entry.code}>
                  <td>
                    <code
                      className={styles.hexCode}
                      onClick={() => copyToClipboard(entry.code)}
                      title="Click to copy"
                    >
                      {formatHex(entry.code)}
                    </code>
                  </td>
                  <td>{entry.humanReadable}</td>
                  <td>
                    <span className={`${styles.badge} ${styles[`badge${entry.category}`]}`}>
                      {entry.category}
                    </span>
                  </td>
                  <td className={styles.countCell}>{entry.count}</td>
                  <td className={styles.dateCell}>{formatDate(entry.lastSeen)}</td>
                  <td className={styles.sessionsCell}>
                    {entry.sessionIds?.length || 0}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {filteredEntries.length === 0 && (
            <div className={styles.emptyState}>
              {enabled ? (
                "No escape codes captured yet. Interact with Claude sessions to generate data."
              ) : (
                "Tracking is disabled. Enable tracking to capture escape codes."
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
