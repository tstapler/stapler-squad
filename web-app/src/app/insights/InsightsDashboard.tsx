// +feature: insights-dashboard
"use client";

import { useState, useMemo, Suspense } from "react";
import dynamic from "next/dynamic";
import { useSearchParams, useRouter } from "next/navigation";
import { useInsightsSummary } from "@/lib/hooks/useInsightsService";
import { useProjectedCost } from "@/lib/hooks/useProjectedCost";
import { useBudgetThreshold } from "@/lib/hooks/useBudgetThreshold";
import { SummaryCards } from "./SummaryCards";
import { TopNTable } from "./TopNTables";
import { SessionsTable } from "./SessionsTable";
import { SessionDetailDrawer } from "./SessionDetailDrawer";
import { ProjectedCostCard } from "./ProjectedCostCard";
import { TimeRangeFilter, resolveTimeRangeDates } from "./TimeRangeFilter";
import type { TimeRangeValue, TimeRangePreset } from "./TimeRangeFilter";
import { InsightsDashboardSkeleton } from "./InsightsDashboardSkeleton";
import { Skeleton } from "@/components/ui/Skeleton";
import type { SessionTokenSummary } from "@/gen/session/v1/insights_pb";
import {
  page,
  pageHeader,
  title,
  subtitle,
  liveIndicator,
  liveDot,
  loadingBanner,
  spinner,
  errorBox,
  emptyState,
  grid2,
  section,
  sectionTitle,
} from "./InsightsDashboard.css";

// Lazy-load recharts and its D3 dependencies (~1.2MB) only when the insights page is visited.
const DailySpendChart = dynamic(
  () => import("./DailySpendChart").then((m) => m.DailySpendChart),
  { ssr: false, loading: () => <Skeleton variant="rectangular" width="100%" height={200} /> }
);
const ModelBreakdownChart = dynamic(
  () => import("./ModelBreakdownChart").then((m) => m.ModelBreakdownChart),
  { ssr: false, loading: () => <Skeleton variant="rectangular" width="100%" height={200} /> }
);
const ModelOverTimeChart = dynamic(
  () => import("./ModelOverTimeChart").then((m) => m.ModelOverTimeChart),
  { ssr: false, loading: () => <Skeleton variant="rectangular" width="100%" height={200} /> }
);

function toLocalDateString(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function InsightsDashboardInner() {
  const router = useRouter();
  const searchParams = useSearchParams();

  const preset = (searchParams.get("preset") ?? "all") as TimeRangePreset;
  const fromParam = searchParams.get("from") ?? undefined;
  const toParam = searchParams.get("to") ?? undefined;

  // Stable Date references — avoid new Date() inline which recreates on every render
  const { from: fromDate, to: toDate } = useMemo(
    () => resolveTimeRangeDates(preset, fromParam, toParam),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [preset, fromParam, toParam]
  );

  const { summary, loading, isLiveUpdating, error } = useInsightsSummary({
    includeOrphans: true,
    from: fromDate,
    to: toDate,
  });

  const projection = useProjectedCost(summary?.daily ?? []);
  const { threshold, setThreshold, isHydrated } = useBudgetThreshold();

  const isOverBudget =
    isHydrated &&
    threshold !== null &&
    threshold > 0 &&
    projection !== null &&
    projection.projectedMonthly > threshold;

  const [selectedSession, setSelectedSession] = useState<SessionTokenSummary | null>(null);

  const timeRangeValue: TimeRangeValue = {
    preset,
    from: fromDate,
    to: toDate,
  };

  function handleTimeRangeChange(v: TimeRangeValue) {
    const params = new URLSearchParams(searchParams.toString());
    params.set("preset", v.preset);
    if (v.preset === "custom") {
      if (v.from) params.set("from", toLocalDateString(v.from));
      else params.delete("from");
      if (v.to) params.set("to", toLocalDateString(v.to));
      else params.delete("to");
    } else {
      params.delete("from");
      params.delete("to");
    }
    router.replace(`?${params.toString()}`);
  }

  return (
    <div className={page}>
      <div className={pageHeader}>
        <div>
          <h1 className={title}>Insights</h1>
          <p className={subtitle}>Token usage analytics and cost breakdown</p>
        </div>
        <div className={liveIndicator} data-live={String(isLiveUpdating)}>
          <div className={liveDot} />
          Live
        </div>
      </div>

      <TimeRangeFilter value={timeRangeValue} onChange={handleTimeRangeChange} />

      {isOverBudget && (
        <div className={errorBox} role="alert">
          Budget alert: projected monthly spend (${projection!.projectedMonthly.toFixed(2)}) exceeds your threshold (${threshold!.toFixed(2)}).
        </div>
      )}

      {error && <div className={errorBox}>{error}</div>}

      {loading && !summary && <InsightsDashboardSkeleton />}

      {summary?.isLoading && (
        <div className={loadingBanner}>
          <div className={spinner} />
          Parsing conversation history in the background…
        </div>
      )}

      {!loading && !error && summary && summary.sessions.length === 0 && (
        <div className={emptyState}>
          No token usage data found. Run some Claude Code sessions to see analytics here.
        </div>
      )}

      {summary && summary.sessions.length > 0 && (
        <>
          <section className={section}>
            <SummaryCards summary={summary} />
            {projection && (
              <ProjectedCostCard
                projection={projection}
                threshold={threshold}
                isHydrated={isHydrated}
                onThresholdChange={setThreshold}
              />
            )}
          </section>

          <section className={section}>
            <div className={grid2}>
              <DailySpendChart daily={summary.daily} />
              <ModelBreakdownChart models={summary.models} />
            </div>
          </section>

          <section className={section}>
            <ModelOverTimeChart daily={summary.daily} mode="cost" />
          </section>

          {(summary.topSkills.length > 0 || summary.topTools.length > 0) && (
            <section className={section}>
              <h2 className={sectionTitle}>Top Usage</h2>
              <div className={grid2}>
                {summary.topSkills.length > 0 && (
                  <TopNTable
                    title="Top Skills"
                    entries={summary.topSkills}
                    valueLabel="Tokens"
                  />
                )}
                {summary.topTools.length > 0 && (
                  <TopNTable
                    title="Top Tools"
                    entries={summary.topTools}
                    valueLabel="Tokens"
                  />
                )}
              </div>
            </section>
          )}

          <section className={section}>
            <h2 className={sectionTitle}>Sessions</h2>
            <SessionsTable
              sessions={summary.sessions}
              onSessionClick={(s) => setSelectedSession(s)}
            />
          </section>
        </>
      )}

      <SessionDetailDrawer
        session={selectedSession}
        onClose={() => setSelectedSession(null)}
      />
    </div>
  );
}

/** Exported wrapper that provides the Suspense boundary required for useSearchParams in App Router. */
export function InsightsDashboard() {
  return (
    <Suspense fallback={<InsightsDashboardSkeleton />}>
      <InsightsDashboardInner />
    </Suspense>
  );
}
