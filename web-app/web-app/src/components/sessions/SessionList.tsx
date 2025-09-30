"use client";

import { useState, useMemo } from "react";
import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import { SessionCard } from "./SessionCard";
import styles from "./SessionList.module.css";

interface SessionListProps {
  sessions: Session[];
  onSessionClick?: (session: Session) => void;
  onDeleteSession?: (sessionId: string) => void;
  onPauseSession?: (sessionId: string) => void;
  onResumeSession?: (sessionId: string) => void;
}

export function SessionList({
  sessions,
  onSessionClick,
  onDeleteSession,
  onPauseSession,
  onResumeSession,
}: SessionListProps) {
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedStatus, setSelectedStatus] = useState<SessionStatus | "all">("all");
  const [selectedCategory, setSelectedCategory] = useState<string | "all">("all");
  const [hidePaused, setHidePaused] = useState(false);

  // Extract unique categories from sessions
  const categories = useMemo(() => {
    const categorySet = new Set<string>();
    sessions.forEach((session) => {
      if (session.category) {
        categorySet.add(session.category);
      }
    });
    return Array.from(categorySet).sort();
  }, [sessions]);

  // Filter sessions based on search query and filters
  const filteredSessions = useMemo(() => {
    return sessions.filter((session) => {
      // Search filter
      if (searchQuery) {
        const query = searchQuery.toLowerCase();
        const matchesSearch =
          session.title.toLowerCase().includes(query) ||
          session.path.toLowerCase().includes(query) ||
          session.branch.toLowerCase().includes(query) ||
          (session.category && session.category.toLowerCase().includes(query));

        if (!matchesSearch) return false;
      }

      // Status filter
      if (selectedStatus !== "all" && session.status !== selectedStatus) {
        return false;
      }

      // Category filter
      if (selectedCategory !== "all" && session.category !== selectedCategory) {
        return false;
      }

      // Hide paused filter
      if (hidePaused && session.status === SessionStatus.PAUSED) {
        return false;
      }

      return true;
    });
  }, [sessions, searchQuery, selectedStatus, selectedCategory, hidePaused]);

  // Group sessions by category
  const groupedSessions = useMemo(() => {
    const grouped = new Map<string, Session[]>();

    filteredSessions.forEach((session) => {
      const category = session.category || "Uncategorized";
      if (!grouped.has(category)) {
        grouped.set(category, []);
      }
      grouped.get(category)!.push(session);
    });

    // Sort categories alphabetically, but keep "Uncategorized" at the end
    const sortedCategories = Array.from(grouped.keys()).sort((a, b) => {
      if (a === "Uncategorized") return 1;
      if (b === "Uncategorized") return -1;
      return a.localeCompare(b);
    });

    return sortedCategories.map((category) => ({
      category,
      sessions: grouped.get(category)!,
    }));
  }, [filteredSessions]);

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h2 className={styles.title}>Sessions ({filteredSessions.length})</h2>

        <div className={styles.filters}>
          {/* Search input */}
          <input
            type="text"
            placeholder="Search sessions..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className={styles.searchInput}
          />

          {/* Status filter */}
          <select
            value={selectedStatus}
            onChange={(e) =>
              setSelectedStatus(
                e.target.value === "all" ? "all" : Number(e.target.value)
              )
            }
            className={styles.select}
          >
            <option value="all">All Statuses</option>
            <option value={SessionStatus.RUNNING}>Running</option>
            <option value={SessionStatus.READY}>Ready</option>
            <option value={SessionStatus.PAUSED}>Paused</option>
            <option value={SessionStatus.LOADING}>Loading</option>
            <option value={SessionStatus.NEEDS_APPROVAL}>
              Needs Approval
            </option>
          </select>

          {/* Category filter */}
          <select
            value={selectedCategory}
            onChange={(e) => setSelectedCategory(e.target.value)}
            className={styles.select}
          >
            <option value="all">All Categories</option>
            {categories.map((category) => (
              <option key={category} value={category}>
                {category}
              </option>
            ))}
          </select>

          {/* Hide paused toggle */}
          <label className={styles.checkbox}>
            <input
              type="checkbox"
              checked={hidePaused}
              onChange={(e) => setHidePaused(e.target.checked)}
            />
            <span>Hide Paused</span>
          </label>
        </div>
      </div>

      {/* Session list */}
      {filteredSessions.length === 0 ? (
        <div className={styles.empty}>
          <p>No sessions found</p>
          {searchQuery && (
            <button
              className={styles.clearButton}
              onClick={() => {
                setSearchQuery("");
                setSelectedStatus("all");
                setSelectedCategory("all");
                setHidePaused(false);
              }}
            >
              Clear filters
            </button>
          )}
        </div>
      ) : (
        <div className={styles.sessionList}>
          {groupedSessions.map(({ category, sessions: categorySessions }) => (
            <div key={category} className={styles.categoryGroup}>
              <h3 className={styles.categoryTitle}>
                {category} ({categorySessions.length})
              </h3>
              <div className={styles.categoryContent}>
                {categorySessions.map((session) => (
                  <SessionCard
                    key={session.id}
                    session={session}
                    onClick={() => onSessionClick?.(session)}
                    onDelete={() => onDeleteSession?.(session.id)}
                    onPause={() => onPauseSession?.(session.id)}
                    onResume={() => onResumeSession?.(session.id)}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
