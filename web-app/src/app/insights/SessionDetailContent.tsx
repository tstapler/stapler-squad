// +feature: insights-dashboard
"use client";

import { useMemo } from "react";
import type { SessionTokenSummary, TurnTokenStat } from "@/gen/session/v1/insights_pb";
import type { BacklogIndexEntry } from "@/lib/hooks/useBacklogService";
import {
  section,
  sectionTitle,
  metaGrid,
  metaLabel,
  metaValue,
  toolsTable,
  toolsTh,
  toolsTd,
  toolsTdRight,
  toolsThRight,
  skillList,
  skillBadge,
  emptyState,
  backlogLink,
  outlierCell,
} from "./SessionDetailDrawer.css";
import { fmtCost, fmtPct, fmtTokens, fmtDate, computeCacheHitRate } from "./insightsFormatters";
import { EstimatedValue } from "@/components/ui/EstimatedValue";
import {
  sortTurnsByTokensDesc,
  computeOutlierThreshold,
  isOutlierTurn,
} from "./turnTimelineUtils";

interface Props {
  session: SessionTokenSummary;
  backlogEntry?: BacklogIndexEntry;
  turns: TurnTokenStat[];
}

/**
 * SessionDetailContent renders a session's Metadata, Backlog Item, Per-Turn
 * Breakdown, Tools Breakdown, and Skill Activations sections. No dialog/route
 * chrome of its own — shared verbatim by SessionDetailDrawer (modal
 * quick-peek) and the /insights/session-detail?sessionId= route's page
 * client, so the two surfaces can't drift apart (Epic 1.4, Story 1.4.2).
 */
export function SessionDetailContent({ session, backlogEntry, turns }: Props) {
  const sortedTurns = useMemo(() => sortTurnsByTokensDesc(turns), [turns]);
  const outlierThreshold = useMemo(() => computeOutlierThreshold(turns), [turns]);

  return (
    <>
      <div className={section}>
        <h3 className={sectionTitle}>Metadata</h3>
        <dl className={metaGrid}>
          <dt className={metaLabel}>Model</dt>
          <dd className={metaValue}>{session.primaryModel || "—"}</dd>

          <dt className={metaLabel}>Project</dt>
          <dd className={metaValue}>{session.projectPath || "—"}</dd>

          <dt className={metaLabel}>Total cost</dt>
          <dd className={metaValue}>{fmtCost(session.estimatedCostUsd)}</dd>

          <dt className={metaLabel}>Message count</dt>
          <dd className={metaValue}>{session.messageCount}</dd>

          <dt className={metaLabel}>Cache hit rate</dt>
          <dd className={metaValue}>{fmtPct(session.cacheHitRate)}</dd>

          <dt className={metaLabel}>Cache writes</dt>
          <dd className={metaValue}>{fmtTokens(session.cacheCreationTokens)}</dd>

          <dt className={metaLabel}>First message</dt>
          <dd className={metaValue}>{fmtDate(session.firstMessageAt)}</dd>

          <dt className={metaLabel}>Last message</dt>
          <dd className={metaValue}>{fmtDate(session.lastMessageAt)}</dd>

          <dt className={metaLabel}>Session ID</dt>
          <dd className={metaValue}>{session.sessionId || "—"}</dd>

          <dt className={metaLabel}>Conversation ID</dt>
          <dd className={metaValue}>{session.conversationId || "—"}</dd>
        </dl>
      </div>

      {backlogEntry && (
        <div className={section} data-testid="backlog-item-section">
          <h3 className={sectionTitle}>Backlog Item</h3>
          <dl className={metaGrid}>
            <dt className={metaLabel}>Title</dt>
            <dd className={metaValue}>
              <a href={`/backlog?item=${backlogEntry.itemId}`} className={backlogLink}>
                {backlogEntry.itemTitle}
              </a>
            </dd>

            <dt className={metaLabel}>Status</dt>
            <dd className={metaValue}>{backlogEntry.itemStatus}</dd>

            <dt className={metaLabel}>Role</dt>
            <dd className={metaValue}>{backlogEntry.sessionRole}</dd>
          </dl>
        </div>
      )}

      <div className={section}>
        <h3 className={sectionTitle}>Per-Turn Breakdown</h3>
        {sortedTurns.length === 0 ? (
          <p className={emptyState}>No per-turn data available for this session.</p>
        ) : (
          <table className={toolsTable}>
            <thead>
              <tr>
                <th className={toolsTh}>Timestamp</th>
                <th className={toolsTh}>Model</th>
                <th className={toolsThRight}>Input</th>
                <th className={toolsThRight}>Output</th>
                <th className={toolsThRight}>Cache</th>
                <th className={toolsTh}>Tools</th>
              </tr>
            </thead>
            <tbody>
              {sortedTurns.map((t, i) => {
                const outlier = isOutlierTurn(t, outlierThreshold);
                return (
                  <tr key={i}>
                    <td className={toolsTd}>{fmtDate(t.timestamp)}</td>
                    <td className={toolsTd}>{t.model || "—"}</td>
                    <td className={toolsTdRight}>
                      <span className={outlier ? outlierCell : undefined}>
                        {t.inputTokens.toString()}
                      </span>
                    </td>
                    <td className={toolsTdRight}>
                      <span className={outlier ? outlierCell : undefined}>
                        {t.outputTokens.toString()}
                      </span>
                    </td>
                    <td
                      className={toolsTdRight}
                      title={`${fmtTokens(t.cacheReadTokens)} read, ${fmtTokens(t.cacheCreationTokens)} written`}
                    >
                      {fmtPct(computeCacheHitRate(Number(t.inputTokens), Number(t.cacheReadTokens)))}
                    </td>
                    <td className={toolsTd}>{t.toolNames.join(", ") || "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      <div className={section}>
        <h3 className={sectionTitle}>Tools Breakdown</h3>
        {session.topTools.length === 0 ? (
          <p className={emptyState}>No tools recorded for this session.</p>
        ) : (
          <table className={toolsTable} data-testid="tools-breakdown-table">
            <thead>
              <tr>
                <th className={toolsTh}>Tool</th>
                <th className={toolsTh}>MCP Server</th>
                <th className={toolsThRight}>Calls</th>
                <th className={toolsThRight}>Cost</th>
              </tr>
            </thead>
            <tbody>
              {session.topTools.map((t, i) => (
                <tr key={i}>
                  <td className={toolsTd}>{t.toolName}</td>
                  <td className={toolsTd}>{t.mcpServer || "—"}</td>
                  <td className={toolsTdRight}>{t.callCount}</td>
                  <td className={toolsTdRight}>
                    {t.costUnpriced ? (
                      "—"
                    ) : t.costMayDoubleCount ? (
                      <EstimatedValue title="Tool-type-level cost attribution: this tool co-occurred with another tool in the same turn, so its cost may double-count that turn's spend.">
                        {fmtCost(t.costUsd)}
                      </EstimatedValue>
                    ) : (
                      fmtCost(t.costUsd)
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className={section}>
        <h3 className={sectionTitle}>Skill Activations</h3>
        {session.skillActivations.length === 0 ? (
          <p className={emptyState}>No skill activations recorded.</p>
        ) : (
          <div className={skillList}>
            {session.skillActivations.map((s, i) => (
              <span key={i} className={skillBadge}>
                {s}
              </span>
            ))}
          </div>
        )}
      </div>
    </>
  );
}
