"use client";
// +feature: escape-analytics

import { useState } from "react";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import { useEscapeAnalyticsSummary, useEscapeEvents } from "@/lib/hooks/useEscapeAnalytics";
import { SequenceHistogram } from "./SequenceHistogram";
import { MangleRateIndicator } from "./MangleRateIndicator";
import { EscapeEventTable } from "./EscapeEventTable";
import * as styles from "./EscapeAnalyticsPage.css";

export function EscapeAnalyticsPage() {
  const sessions = useAppSelector(selectAllSessions);
  const [selectedSessionId, setSelectedSessionId] = useState<string>("");
  const [stageFilter, setStageFilter] = useState("");
  const [sequenceTypeFilter, setSequenceTypeFilter] = useState("");
  const [mangledOnly, setMangledOnly] = useState(false);

  const {
    histogram,
    totalSequences,
    totalMangled,
    mangleRate,
    loading: summaryLoading,
    error: summaryError,
  } = useEscapeAnalyticsSummary(selectedSessionId);

  const {
    events,
    nextPageToken,
    loading: eventsLoading,
    error: eventsError,
    fetchNextPage,
  } = useEscapeEvents(selectedSessionId, {
    stage: stageFilter,
    sequenceType: sequenceTypeFilter,
    mangledOnly,
    pageSize: 50,
  });

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.title}>Escape Analytics</h1>
        <p className={styles.subtitle}>
          Inspect terminal escape sequence statistics and mangle events per session.
        </p>
      </div>

      <div className={styles.sessionSelectorRow}>
        <label className={styles.selectorLabel} htmlFor="session-selector">
          Session:
        </label>
        <select
          id="session-selector"
          className={styles.sessionSelect}
          value={selectedSessionId}
          onChange={(e) => setSelectedSessionId(e.target.value)}
          aria-label="Select session for escape analytics"
        >
          <option value="">— select a session —</option>
          {sessions.map((session) => (
            <option key={session.id} value={session.id}>
              {session.title || session.id}
            </option>
          ))}
        </select>
      </div>

      {!selectedSessionId && (
        <p className={styles.noSessionMessage}>
          Select a session above to view escape analytics.
        </p>
      )}

      {selectedSessionId && (
        <>
          {summaryError && (
            <div className={styles.errorBanner} role="alert">
              Failed to load summary: {summaryError.message}
            </div>
          )}

          <div className={styles.grid}>
            <div className={styles.card}>
              <h2 className={styles.cardTitle}>Mangle Rate</h2>
              {summaryLoading ? (
                <p className={styles.loadingText}>Loading…</p>
              ) : (
                <MangleRateIndicator
                  mangleRate={mangleRate}
                  totalSequences={totalSequences}
                  totalMangled={totalMangled}
                />
              )}
            </div>

            <div className={styles.card}>
              <h2 className={styles.cardTitle}>Sequence Histogram</h2>
              {summaryLoading ? (
                <p className={styles.loadingText}>Loading…</p>
              ) : (
                <SequenceHistogram histogram={histogram} />
              )}
            </div>
          </div>

          <div className={styles.fullWidthCard}>
            <h2 className={styles.cardTitle}>Escape Events</h2>

            <div className={styles.filterRow}>
              <input
                className={styles.filterInput}
                type="text"
                placeholder="Filter by stage…"
                value={stageFilter}
                onChange={(e) => setStageFilter(e.target.value)}
                aria-label="Filter by stage"
              />
              <input
                className={styles.filterInput}
                type="text"
                placeholder="Filter by sequence type…"
                value={sequenceTypeFilter}
                onChange={(e) => setSequenceTypeFilter(e.target.value)}
                aria-label="Filter by sequence type"
              />
              <label className={styles.filterLabel}>
                <input
                  type="checkbox"
                  checked={mangledOnly}
                  onChange={(e) => setMangledOnly(e.target.checked)}
                  aria-label="Show mangled events only"
                />
                Mangled only
              </label>
            </div>

            {eventsError && (
              <div className={styles.errorBanner} role="alert">
                Failed to load events: {eventsError.message}
              </div>
            )}

            <EscapeEventTable
              events={events}
              loading={eventsLoading}
              onLoadMore={fetchNextPage}
              hasMore={!!nextPageToken}
            />
          </div>
        </>
      )}
    </div>
  );
}
