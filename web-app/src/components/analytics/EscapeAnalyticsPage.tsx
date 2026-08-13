"use client";
// +feature: escape-analytics

import { useRef, useState } from "react";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import {
  useEscapeAnalyticsSummary,
  useEscapeAnalyticsGlobalSummary,
  useEscapeEvents,
} from "@/lib/hooks/useEscapeAnalytics";
import { SequenceHistogram } from "./SequenceHistogram";
import { MangleRateIndicator } from "./MangleRateIndicator";
import { EscapeEventTable } from "./EscapeEventTable";
import { SessionEscapeBreakdownTable } from "./SessionEscapeBreakdownTable";
import * as styles from "./EscapeAnalyticsPage.css";

type ViewMode = "per_session" | "all_sessions";

const TABS: { id: ViewMode; label: string }[] = [
  { id: "per_session", label: "Per-Session" },
  { id: "all_sessions", label: "All Sessions" },
];

// Dominant-contributor threshold (ux.md §3): informational only, shown when a
// single session accounts for more than half of all mangled sequences fleet-wide.
const DOMINANT_CONTRIBUTOR_SHARE_THRESHOLD = 0.5;

export function EscapeAnalyticsPage() {
  const sessions = useAppSelector(selectAllSessions);
  const [selectedSessionId, setSelectedSessionId] = useState<string>("");
  const [stageFilter, setStageFilter] = useState("");
  const [sequenceTypeFilter, setSequenceTypeFilter] = useState("");
  const [mangledOnly, setMangledOnly] = useState(false);
  const [viewMode, setViewMode] = useState<ViewMode>("per_session");
  const tabRefs = useRef<Record<ViewMode, HTMLButtonElement | null>>({
    per_session: null,
    all_sessions: null,
  });

  const perSessionActive = viewMode === "per_session";
  const allSessionsActive = viewMode === "all_sessions";

  const {
    histogram,
    totalSequences,
    totalMangled,
    mangleRate,
    loading: summaryLoading,
    error: summaryError,
  } = useEscapeAnalyticsSummary(selectedSessionId, perSessionActive);

  const {
    events,
    nextPageToken,
    loading: eventsLoading,
    error: eventsError,
    fetchNextPage,
  } = useEscapeEvents(perSessionActive ? selectedSessionId : "", {
    stage: stageFilter,
    sequenceType: sequenceTypeFilter,
    mangledOnly,
    pageSize: 50,
  });

  const {
    histogram: globalHistogram,
    totalSequences: globalTotalSequences,
    totalMangled: globalTotalMangled,
    mangleRate: globalMangleRate,
    perSession,
    loading: globalLoading,
    error: globalError,
  } = useEscapeAnalyticsGlobalSummary(allSessionsActive);

  const dominantContributor = perSession.reduce<
    (typeof perSession)[number] | null
  >((max, row) => (!max || row.totalMangled > max.totalMangled ? row : max), null);
  const dominantContributorShare =
    dominantContributor && globalTotalMangled > 0n
      ? Number(dominantContributor.totalMangled) / Number(globalTotalMangled)
      : 0;
  const showDominantContributor =
    !!dominantContributor &&
    dominantContributorShare > DOMINANT_CONTRIBUTOR_SHARE_THRESHOLD;

  const handleTabKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    const currentIndex = TABS.findIndex((tab) => tab.id === viewMode);
    const delta = event.key === "ArrowRight" ? 1 : -1;
    const nextIndex = (currentIndex + delta + TABS.length) % TABS.length;
    const nextTab = TABS[nextIndex];
    setViewMode(nextTab.id);
    tabRefs.current[nextTab.id]?.focus();
  };

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.title}>Escape Analytics</h1>
        <p className={styles.subtitle}>
          Inspect terminal escape sequence statistics and mangle events per session, or across
          the whole fleet.
        </p>
      </div>

      <div className={styles.tablist} role="tablist" aria-label="Escape analytics view">
        {TABS.map((tab) => {
          const selected = tab.id === viewMode;
          return (
            <button
              key={tab.id}
              ref={(el) => {
                tabRefs.current[tab.id] = el;
              }}
              id={`tab-${tab.id}`}
              role="tab"
              type="button"
              className={styles.tab}
              aria-selected={selected}
              aria-controls={`tabpanel-${tab.id}`}
              tabIndex={selected ? 0 : -1}
              onClick={() => setViewMode(tab.id)}
              onKeyDown={handleTabKeyDown}
              data-testid={`tab-${tab.id}`}
            >
              {tab.label}
            </button>
          );
        })}
      </div>

      {perSessionActive && (
        <div
          id="tabpanel-per_session"
          role="tabpanel"
          aria-labelledby="tab-per_session"
          className={styles.tabpanel}
        >
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
      )}

      {allSessionsActive && (
        <div
          id="tabpanel-all_sessions"
          role="tabpanel"
          aria-labelledby="tab-all_sessions"
          className={styles.tabpanel}
        >
          {globalError && (
            <div className={styles.errorBanner} role="alert">
              Failed to load global summary: {globalError.message}
            </div>
          )}

          {!globalError && globalLoading && (
            <p className={styles.loadingText}>Loading…</p>
          )}

          {!globalError && !globalLoading && globalTotalSequences === 0n && (
            <p className={styles.noSessionMessage}>
              No escape sequence events recorded across any session yet.
            </p>
          )}

          {!globalError && !globalLoading && globalTotalSequences > 0n && (
            <>
              <div className={styles.grid}>
                <div className={styles.card}>
                  <h2 className={styles.cardTitle}>Fleet-Wide Mangle Rate</h2>
                  <MangleRateIndicator
                    mangleRate={globalMangleRate}
                    totalSequences={globalTotalSequences}
                    totalMangled={globalTotalMangled}
                  />
                  {showDominantContributor && dominantContributor && (
                    <p className={styles.subtitle}>
                      Session {dominantContributor.sessionId} accounts for the majority of
                      mangled sequences fleet-wide.
                    </p>
                  )}
                </div>

                <div className={styles.card}>
                  <h2 className={styles.cardTitle}>Sequence Histogram</h2>
                  <SequenceHistogram histogram={globalHistogram} />
                </div>
              </div>

              <div className={styles.fullWidthCard}>
                <h2 className={styles.cardTitle}>Per-Session Breakdown</h2>
                <SessionEscapeBreakdownTable
                  rows={perSession}
                  fleetMangleRate={globalMangleRate}
                  loading={globalLoading}
                />
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}
