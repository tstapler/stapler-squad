// +feature: insights-dashboard
"use client";

import type { GetInsightsSummaryResponse } from "@/gen/session/v1/insights_pb";
import { grid, card, cardLabel, cardValue, cardSub } from "./SummaryCards.css";

interface Props {
  summary: GetInsightsSummaryResponse;
}

function fmtCost(usd: number): string {
  if (usd < 0.01) return `$${usd.toFixed(4)}`;
  if (usd < 1) return `$${usd.toFixed(3)}`;
  return `$${usd.toFixed(2)}`;
}

function fmtTokens(n: bigint): string {
  const num = Number(n);
  if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`;
  if (num >= 1_000) return `${(num / 1_000).toFixed(1)}K`;
  return num.toString();
}

function fmtPct(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`;
}

export function SummaryCards({ summary }: Props) {
  const sessionCount = summary.sessions.length;
  const orphanCount = summary.sessions.filter((s) => s.isOrphan).length;

  return (
    <div className={grid}>
      <div className={card}>
        <span className={cardLabel}>Total Cost</span>
        <span className={cardValue}>{fmtCost(summary.totalCostUsd)}</span>
        <span className={cardSub}>{sessionCount} session{sessionCount !== 1 ? "s" : ""}</span>
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
        <span className={cardSub}>{fmtTokens(summary.totalCacheReadTokens)} cached</span>
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
