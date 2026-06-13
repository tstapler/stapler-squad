"use client";

import { useRef } from "react";
import { Session, SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";
import { Tooltip } from "../ui/Tooltip";
import { SessionActionsOverflow, SessionActionsOverflowHandle } from "./SessionActionsOverflow";
import { SubStatusChip } from "./SubStatusChip";
import {
  row,
  rowPaused,
  rowActive,
  statusDot,
  nameCell as nameCellStyle,
  name as nameStyle,
  agentIcon as agentIconStyle,
  path as pathStyle,
  elapsed as elapsedStyle,
  actions as actionsStyle,
  memoryBadge,
  memoryBadgeWarning,
  memoryBadgeHigh,
  diffBadge,
  branchCell,
  rowMemoryPressure,
} from "./SessionRow.css";
import { ColumnKey, DEFAULT_VISIBLE_COLUMNS, buildRowGridTemplate } from "./session-columns";

interface SessionRowProps {
  session: Session;
  onClick?: () => void;
  onPause?: () => void;
  onResume?: () => void;
  onDelete?: () => Promise<void> | void;
  onClone?: () => void;
  onOpenInNewPane?: () => void;
  onNewWorkspace?: () => void;
  onRestart?: (sessionId: string) => Promise<boolean | void>;
  onCreateCheckpoint?: (sessionId: string, label: string) => Promise<boolean>;
  onRunOneShot?: (sessionId: string) => Promise<void>;
  onSetRateLimitEnabled?: (sessionId: string, enabled: boolean) => void;
  onClearConversationState?: (sessionId: string) => Promise<boolean>;
  onUpdateTags?: (sessionId: string, tags: string[]) => void;
  onHibernate?: () => void;
  onResumeFromHibernation?: () => void;
  /** When true, hides the Needs Approval SubStatusChip during optimistic clear */
  suppressApprovalSubStatus?: boolean;
  /** Which optional columns to render. Defaults to DEFAULT_VISIBLE_COLUMNS. */
  visibleColumns?: ColumnKey[];
}

function getStatusDotValue(status: SessionStatus): string {
  switch (status) {
    case SessionStatus.ACTIVE:  // includes legacy RUNNING (same wire value = 1)
      return "running";
    case SessionStatus.READY:
      return "idle";
    case SessionStatus.PAUSED:
      return "paused-session";
    case SessionStatus.STOPPED:
      return "paused";
    case SessionStatus.LOADING:
    case SessionStatus.CREATING:
      return "loading";
    case SessionStatus.NEEDS_APPROVAL:
      return "needs-approval";
    case SessionStatus.HIBERNATED:
      return "hibernated";
    default:
      return "idle";
  }
}

function formatElapsed(ts?: { seconds: bigint; nanos: number }): string {
  if (!ts || ts.seconds === BigInt(0)) return "";
  const now = Date.now();
  const date = new Date(Number(ts.seconds) * 1000);
  const seconds = Math.floor((now - date.getTime()) / 1000);

  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
  return `${Math.floor(seconds / 86400)}d`;
}

function getAgentEmoji(program: string): string {
  const p = program.toLowerCase();
  if (p.includes("claude")) return "✦";
  if (p.includes("aider")) return "⚡";
  if (p.includes("cursor")) return "◎";
  if (p.includes("copilot")) return "◈";
  if (p.includes("gpt") || p.includes("openai")) return "◉";
  if (p.includes("gemini")) return "◆";
  if (p.includes("agy") || p.includes("antigravity")) return "◆";
  return "◇";
}

function getLastActivity(session: Session): { seconds: bigint; nanos: number } | undefined {
  const moSecs = session.lastMeaningfulOutput?.seconds ?? BigInt(0);
  const tuSecs = session.lastTerminalUpdate?.seconds ?? BigInt(0);
  if (moSecs === BigInt(0) && tuSecs === BigInt(0)) return undefined;
  return moSecs >= tuSecs ? session.lastMeaningfulOutput : session.lastTerminalUpdate;
}

export function SessionRow({
  session, onClick,
  onPause, onResume, onDelete,
  onClone, onOpenInNewPane, onNewWorkspace,
  onRestart, onCreateCheckpoint, onRunOneShot,
  onSetRateLimitEnabled, onClearConversationState, onUpdateTags,
  onHibernate, onResumeFromHibernation,
  suppressApprovalSubStatus = false,
  visibleColumns = DEFAULT_VISIBLE_COLUMNS,
}: SessionRowProps) {
  const overflowRef = useRef<SessionActionsOverflowHandle>(null);

  const dotStatus = getStatusDotValue(session.status);
  const isPaused = session.status === SessionStatus.PAUSED;
  const lastActivity = getLastActivity(session);
  const elapsedText = formatElapsed(lastActivity ?? session.updatedAt);
  // Show branch separately if the branch column is visible; otherwise fold into displayName.
  const showBranchCol = visibleColumns.includes("branch");
  const displayName = showBranchCol ? session.title : (session.branch || session.title);

  const memMB = Number(session.memoryRssMb ?? 0n);
  const memorySeverityClass =
    memMB > 500 ? memoryBadgeHigh :
    memMB > 300 ? memoryBadgeWarning : "";

  const handleContextMenu = (e: React.MouseEvent<HTMLLIElement>) => {
    e.preventDefault();
    overflowRef.current?.openAt(e.clientX, e.clientY);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLLIElement>) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onClick?.();
    }
  };

  return (
    <li
      className={[
        row,
        memMB > 500 ? rowMemoryPressure : "",
        isPaused ? rowPaused : "",
        session.status === SessionStatus.ACTIVE && session.subStatus === SubStatus.PROCESSING ? rowActive : "",
      ].filter(Boolean).join(" ")}
      style={{ gridTemplateColumns: buildRowGridTemplate(visibleColumns) }}
      data-testid="session-row"
      data-paused={isPaused ? "true" : undefined}
      onClick={onClick}
      onContextMenu={handleContextMenu}
      onKeyDown={handleKeyDown}
      tabIndex={0}
      aria-label={`Session ${session.title}, status: ${isPaused ? "paused" : dotStatus}, program: ${session.program}`}
    >
      {/* Status dot — always visible */}
      <Tooltip label={`Status: ${dotStatus}`}>
        <span
          className={statusDot}
          data-status={dotStatus}
          role="img"
          aria-label={`Status: ${dotStatus}`}
        />
      </Tooltip>

      {/* Name + path stacked — always visible */}
      <span className={nameCellStyle}>
        <span style={{ display: "flex", alignItems: "center", gap: "6px", minWidth: 0 }}>
          <span className={nameStyle} aria-label={displayName} title={displayName}>
            {displayName}
          </span>
          {session.status === SessionStatus.ACTIVE &&
            session.subStatus !== SubStatus.UNSPECIFIED &&
            !(suppressApprovalSubStatus && session.subStatus === SubStatus.NEEDS_APPROVAL) && (
              <SubStatusChip subStatus={session.subStatus} />
            )}
        </span>
        {session.path && (
          <Tooltip label={session.path} side="bottom">
            <span className={pathStyle} aria-label={`Path: ${session.path}`}>
              {session.path}
            </span>
          </Tooltip>
        )}
      </span>

      {/* Agent icon — optional column */}
      {visibleColumns.includes("agent") && (
        <span
          className={agentIconStyle}
          title={session.program}
          aria-label={`Agent: ${session.program}`}
        >
          {getAgentEmoji(session.program)}
        </span>
      )}

      {/* Diff stats — optional column */}
      {visibleColumns.includes("diff") && (
        <span className={diffBadge} aria-label="Diff stats">
          {session.diffStats ? (
            <>
              <span style={{ color: "var(--success)" }}>+{session.diffStats.added}</span>
              {" "}
              <span style={{ color: "var(--error)" }}>-{session.diffStats.removed}</span>
            </>
          ) : (
            <span style={{ opacity: 0.3 }}>—</span>
          )}
        </span>
      )}

      {/* Memory usage — optional column, colored by severity */}
      {visibleColumns.includes("memory") && (
        <span
          className={[memoryBadge, memorySeverityClass].filter(Boolean).join(" ")}
          title={memMB > 0 ? `Process RSS: ${memMB} MB` : undefined}
          aria-label={memMB > 0 ? `${memMB} MB RAM` : undefined}
          aria-hidden={memMB === 0 ? "true" : undefined}
        >
          {memMB > 0
            ? memMB >= 1024
              ? `${(memMB / 1024).toFixed(1)} GB`
              : `${memMB} MB`
            : ""}
        </span>
      )}

      {/* Branch — optional column */}
      {visibleColumns.includes("branch") && (
        <span
          className={branchCell}
          title={session.branch || undefined}
          aria-label={session.branch ? `Branch: ${session.branch}` : "No branch"}
        >
          {session.branch || <span style={{ opacity: 0.3 }}>—</span>}
        </span>
      )}

      {/* Elapsed time — optional column */}
      {visibleColumns.includes("elapsed") && (
        <time
          className={elapsedStyle}
          dateTime={lastActivity ? new Date(Number(lastActivity.seconds) * 1000).toISOString() : undefined}
          title={lastActivity ? new Date(Number(lastActivity.seconds) * 1000).toLocaleString() : undefined}
        >
          {elapsedText}
        </time>
      )}

      {/* Actions — always visible */}
      <span className={actionsStyle} aria-label="Session actions">
        <SessionActionsOverflow
          ref={overflowRef}
          session={session}
          showPrimaryAction
          onPause={onPause}
          onResume={onResume}
          onHibernate={onHibernate}
          onResumeFromHibernation={onResumeFromHibernation}
          onDelete={onDelete}
          onClone={onClone}
          onOpenInNewPane={onOpenInNewPane}
          onNewWorkspace={onNewWorkspace}
          onRestart={onRestart}
          onCreateCheckpoint={onCreateCheckpoint}
          onRunOneShot={onRunOneShot}
          onSetRateLimitEnabled={onSetRateLimitEnabled}
          onClearConversationState={onClearConversationState}
          onUpdateTags={onUpdateTags}
        />
      </span>
    </li>
  );
}
