// +feature: insights-dashboard
"use client";

import { useEffect } from "react";
import { createPortal } from "react-dom";
import type { SessionTokenSummary } from "@/gen/session/v1/insights_pb";
import {
  overlay,
  drawer,
  drawerHeader,
  drawerTitle,
  sessionIdChip,
  closeButton,
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
  srOnly,
} from "./SessionDetailDrawer.css";
import { fmtCost, fmtPct, fmtDate, shortId } from "./insightsFormatters";

interface Props {
  session: SessionTokenSummary | null;
  onClose: () => void;
}

export function SessionDetailDrawer({ session, onClose }: Props) {
  useEffect(() => {
    if (!session) return;
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [session, onClose]);

  if (!session || typeof document === "undefined") return null;

  const displayId = session.sessionId || session.conversationId;

  const content = (
    <>
      <div className={overlay} onClick={onClose} aria-hidden="true" />
      <div
        className={drawer}
        role="dialog"
        aria-modal="true"
        aria-label="Session details"
        aria-describedby="session-detail-description"
      >
        <div id="session-detail-description" className={srOnly}>
          Session token usage details including cost, model, tools used, and skill activations.
        </div>
        <div className={drawerHeader}>
          <div className={drawerTitle}>
            <span className={sessionIdChip}>{shortId(displayId)}</span>
            Session Details
          </div>
          <button
            type="button"
            className={closeButton}
            onClick={onClose}
            aria-label="Close session details"
          >
            ×
          </button>
        </div>

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

        <div className={section}>
          <h3 className={sectionTitle}>Tools Breakdown</h3>
          {session.topTools.length === 0 ? (
            <p className={emptyState}>No tools recorded for this session.</p>
          ) : (
            <table className={toolsTable}>
              <thead>
                <tr>
                  <th className={toolsTh}>Tool</th>
                  <th className={toolsTh}>MCP Server</th>
                  <th className={toolsThRight}>Calls</th>
                </tr>
              </thead>
              <tbody>
                {session.topTools.map((t, i) => (
                  <tr key={i}>
                    <td className={toolsTd}>{t.toolName}</td>
                    <td className={toolsTd}>{t.mcpServer || "—"}</td>
                    <td className={toolsTdRight}>{t.callCount}</td>
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
                <span key={i} className={skillBadge}>{s}</span>
              ))}
            </div>
          )}
        </div>
      </div>
    </>
  );

  return createPortal(content, document.body);
}
