// +feature: insights-dashboard
"use client";

import type { GetInsightsSummaryResponse } from "@/gen/session/v1/insights_pb";
import { grid, card, cardLabel, cardValue, cardSub } from "./SummaryCards.css";
import { fmtCost, fmtTokens, fmtPct } from "./insightsFormatters";

interface Props {
  summary: GetInsightsSummaryResponse;
}

export function SummaryCards({ summary }: Props) {
  const sessionCount = summary.sessions.length;
  const orphanCount = summary.sessions.filter((s) => s.isOrphan).length;
  const totalCacheCreationTokens = summary.sessions.reduce(
    (sum, s) => sum + s.cacheCreationTokens,
    0n,
  );

  return (
    <div className={grid}>
      <div className={card}>
        <span className={cardLabel}>Total Cost</span>
        <span className={cardValue}>{fmtCost(summary.totalCostUsd)}</span>
        <span className={cardSub}>{sessionCount} session{sessionCount !== 1 ? "s" : ""}</span>
        {summary.unpricedModels.length > 0 && (
          <span className={cardSub}>
            excludes {summary.unpricedModels.length} unpriced model{summary.unpricedModels.length !== 1 ? "s" : ""}
          </span>
        )}
      </div>

      <div className={card}>
        <span className={cardLabel}>Input Tokens</span>
        <span className={cardValue}>{fmtTokens(summary.totalInputTokens)}</span>
        <span className={cardSub}>total input</span>
      </div>

      <div className={card}>
        <span className={cardLabel}>Output Tokens</span>
        <span className={cardValue}>{fmtTokens(summary.totalOutputTokens)}</span>
        <span className={cardSub}>total output</span>
      </div>

      <div className={card}>
        <span className={cardLabel}>Cache Hit Rate</span>
        <span className={cardValue}>{fmtPct(summary.overallCacheHitRate)}</span>
        <span className={cardSub}>{fmtTokens(summary.totalCacheReadTokens)} read</span>
      </div>

      <div className={card}>
        <span className={cardLabel}>Cache Writes</span>
        <span className={cardValue}>{fmtTokens(totalCacheCreationTokens)}</span>
        <span className={cardSub}>tokens written to cache</span>
      </div>

      {orphanCount > 0 && (
        <div className={card}>
          <span className={cardLabel}>Orphaned</span>
          <span className={cardValue}>{orphanCount}</span>
          <span className={cardSub}>unlinked sessions</span>
        </div>
      )}
    </div>
  );
}
