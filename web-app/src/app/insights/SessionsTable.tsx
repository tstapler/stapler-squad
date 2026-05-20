// +feature: insights-dashboard
"use client";

import { useState } from "react";
import type { SessionTokenSummary } from "@/gen/session/v1/insights_pb";
import {
  tableCard,
  tableHeader,
  tableTitle,
  orphanToggle,
  table,
  th,
  thRight,
  td,
  tdRight,
  tdMono,
  orphanBadge,
  empty,
} from "./SessionsTable.css";

interface Props {
  sessions: SessionTokenSummary[];
}

function fmtCost(usd: number): string {
  if (usd < 0.001) return `$${usd.toFixed(5)}`;
  if (usd < 0.01) return `$${usd.toFixed(4)}`;
  return `$${usd.toFixed(3)}`;
}

function fmtTokens(n: bigint): string {
  const num = Number(n);
  if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`;
  if (num >= 1_000) return `${(num / 1_000).toFixed(1)}K`;
  return num.toString();
}

function fmtPct(rate: number): string {
  return `${(rate * 100).toFixed(0)}%`;
}

function shortId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) + "…" : id;
}

function pathBasename(p: string): string {
  return p.split("/").pop() || p;
}

export function SessionsTable({ sessions }: Props) {
  const [showOrphans, setShowOrphans] = useState(true);

  const orphanCount = sessions.filter((s) => s.isOrphan).length;
  const displayed = showOrphans ? sessions : sessions.filter((s) => !s.isOrphan);

  return (
    <div className={tableCard}>
      <div className={tableHeader}>
        <div className={tableTitle}>Sessions ({displayed.length})</div>
        {orphanCount > 0 && (
          <button
            type="button"
            className={orphanToggle}
            onClick={() => setShowOrphans((v) => !v)}
          >
            {showOrphans ? "Hide" : "Show"} orphans ({orphanCount})
          </button>
        )}
      </div>

      {displayed.length === 0 ? (
        <div className={empty}>No sessions</div>
      ) : (
        <table className={table}>
          <thead>
            <tr>
              <th className={th}>Session</th>
              <th className={th}>Model</th>
              <th className={th}>Path</th>
              <th className={thRight}>Input</th>
              <th className={thRight}>Output</th>
              <th className={thRight}>Cache</th>
              <th className={thRight}>Cost</th>
            </tr>
          </thead>
          <tbody>
            {displayed.map((s) => (
              <tr key={s.conversationId || s.sessionId}>
                <td className={tdMono} title={s.sessionId || s.conversationId}>
                  {s.isOrphan ? (
                    <>
                      {shortId(s.conversationId)}
                      <span className={orphanBadge}>orphan</span>
                    </>
                  ) : (
                    shortId(s.sessionId || s.conversationId)
                  )}
                </td>
                <td className={td} title={s.primaryModel}>{s.primaryModel || "—"}</td>
                <td className={td} title={s.projectPath}>{pathBasename(s.projectPath) || "—"}</td>
                <td className={tdRight}>{fmtTokens(s.totalInputTokens)}</td>
                <td className={tdRight}>{fmtTokens(s.totalOutputTokens)}</td>
                <td className={tdRight}>{fmtPct(s.cacheHitRate)}</td>
                <td className={tdRight}>{fmtCost(s.estimatedCostUsd)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
