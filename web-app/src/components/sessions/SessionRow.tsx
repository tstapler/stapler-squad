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
  pathLine as pathLineStyle,
  elapsed as elapsedStyle,
  elapsedIcon as elapsedIconStyle,
  actions as actionsStyle,
  primaryActionWrapper,
  inlineActionButton,
  rowOverflowButton,
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
  onToggleAutonomousMode?: (sessionId: string, enabled: boolean) => void;
  onSteerAutonomousSession?: (sessionId: string, message: string) => void;
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

const STATUS_DOT_LABELS: Record<string, string> = {
  "running": "Running",
  "idle": "Idle",
  "paused-session": "Paused",
  "paused": "Stopped",
  "loading": "Loading",
  "needs-approval": "Needs Approval",
  "hibernated": "Hibernated",
};

function getStatusDotLabel(dotValue: string): string {
  return STATUS_DOT_LABELS[dotValue] ?? dotValue;
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

function abbreviatePath(p: string): string {
  return p.replace(/^\/home\/[^/]+\//, "~/").replace(/^\/Users\/[^/]+\//, "~/");
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
  onSetRateLimitEnabled, onToggleAutonomousMode, onSteerAutonomousSession, onClearConversationState, onUpdateTags,
  onHibernate, onResumeFromHibernation,
  suppressApprovalSubStatus = false,
  visibleColumns = DEFAULT_VISIBLE_COLUMNS,
}: SessionRowProps) {
  const overflowRef = useRef<SessionActionsOverflowHandle>(null);

  const dotStatus = getStatusDotValue(session.status);
  const isPaused = session.status === SessionStatus.PAUSED;
  const isNeedsApproval = session.status === SessionStatus.NEEDS_APPROVAL;
  const isHibernated = session.status === SessionStatus.HIBERNATED;
  const isRunning = session.status === SessionStatus.ACTIVE;
  const isCreating = session.status === SessionStatus.CREATING;
  // Sessions needing user attention always show their primary action
  const actionsAlwaysVisible = isPaused || isNeedsApproval || isHibernated;
  const lastActivity = getLastActivity(session);
  const elapsedText = formatElapsed(lastActivity ?? session.updatedAt);
  // Show branch separately if the branch column is visible; otherwise fold into displayName.
  const showBranchCol = visibleColumns.includes("branch");
  const displayName = showBranchCol ? session.title : (session.branch || session.title);

  const memMB = Number(session.memoryRssMb ?? 0n);
  const memorySeverityClass =
    memMB > 500 ? memoryBadgeHigh :
    memMB > 300 ? memoryBadgeWarning : "";

  const handleContextMenu = (e: React.MouseEvent<HTMLDivElement>) => {
    e.preventDefault();
    overflowRef.current?.openAt(e.clientX, e.clientY);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if ((e.key === "Enter" || e.key === " ") && !(e.target instanceof HTMLButtonElement) && !(e.target instanceof HTMLInputElement) && !(e.target instanceof HTMLTextAreaElement) && !(e.target instanceof HTMLAnchorElement) && !(e.target instanceof HTMLSelectElement)) {
      e.preventDefault();
      onClick?.();
    }
  };

  return (
    <div
      className={[
        row,
        memMB > 500 ? rowMemoryPressure : "",
        isPaused ? rowPaused : "",
        session.status === SessionStatus.ACTIVE &&
          (session.subStatus === SubStatus.PROCESSING || session.subStatus === SubStatus.WAITING_FOR_AGENT)
          ? rowActive
          : "",
      ].filter(Boolean).join(" ")}
      style={{ gridTemplateColumns: buildRowGridTemplate(visibleColumns) }}
      data-testid="session-row"
      data-paused={isPaused ? "true" : undefined}
      data-actions-visible={actionsAlwaysVisible ? "true" : undefined}
      onClick={onClick}
      onContextMenu={handleContextMenu}
      onKeyDown={handleKeyDown}
      tabIndex={0}
      aria-label={`Session ${session.title}, status: ${getStatusDotLabel(dotStatus)}, program: ${session.program}${session.path ? `, path: ${abbreviatePath(session.path)}` : ""}`}
    >
      {/* Status dot — always visible */}
      <Tooltip label={`Status: ${getStatusDotLabel(dotStatus)}`}>
        <span
          className={statusDot}
          data-status={dotStatus}
          aria-hidden="true"
        />
      </Tooltip>

      {/* Name + path stacked — always visible */}
      <span className={nameCellStyle}>
        <span className={nameStyle} title={displayName}>
          {displayName}
        </span>
        <span className={pathLineStyle}>
          {session.path && (
            <Tooltip label={session.path} side="bottom">
              <span className={pathStyle} role="img" aria-label={`Path: ${session.path}`}>
                {abbreviatePath(session.path)}
              </span>
            </Tooltip>
          )}
          {session.status === SessionStatus.ACTIVE &&
            session.subStatus !== SubStatus.UNSPECIFIED &&
            session.subStatus !== SubStatus.READY &&
            session.subStatus !== SubStatus.IDLE &&
            !(suppressApprovalSubStatus && (session.subStatus === SubStatus.NEEDS_APPROVAL || session.subStatus === SubStatus.INPUT_REQUIRED)) && (
              <SubStatusChip subStatus={session.subStatus} />
            )}
        </span>
      </span>

      {/* Agent icon — optional column */}
      {visibleColumns.includes("agent") && (
        <span
          className={agentIconStyle}
          role="img"
          title={session.program}
          aria-label={`Agent: ${session.program}`}
        >
          {getAgentEmoji(session.program)}
        </span>
      )}

      {/* Diff stats — optional column */}
      {visibleColumns.includes("diff") && (
        <span
          className={diffBadge}
          role={session.diffStats && (session.diffStats.added > 0 || session.diffStats.removed > 0) ? "img" : undefined}
          aria-label={session.diffStats && (session.diffStats.added > 0 || session.diffStats.removed > 0)
            ? `Diff: +${session.diffStats.added} -${session.diffStats.removed}`
            : undefined}
          aria-hidden={session.diffStats && (session.diffStats.added > 0 || session.diffStats.removed > 0) ? undefined : "true"}
        >
          {session.diffStats && (session.diffStats.added > 0 || session.diffStats.removed > 0) ? (
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
          role="img"
          title={memMB > 0 ? `Process RSS: ${memMB} MB` : undefined}
          aria-label={memMB > 0 ? `${memMB} MB RAM` : "No memory data"}
        >
          {memMB > 0
            ? memMB >= 1024
              ? `${(memMB / 1024).toFixed(1)} GB`
              : `${memMB} MB`
            : <span style={{ opacity: 0.3 }}>—</span>}
        </span>
      )}

      {/* Branch — optional column */}
      {visibleColumns.includes("branch") && (
        <span
          className={branchCell}
          role="img"
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
          aria-label={elapsedText ? `Last active: ${elapsedText}` : "No recent activity"}
        >
          {elapsedText
            ? <><span className={elapsedIconStyle} aria-hidden="true">⏱</span>{elapsedText}</>
            : <span style={{ opacity: 0.3 }}>—</span>}
        </time>
      )}

      {/* Actions: primary (hover-only unless needs attention) + overflow (always visible) */}
      <span className={actionsStyle} role="group" aria-label="Session actions">
        <span className={primaryActionWrapper} role="presentation">
          {(isPaused || isNeedsApproval) && onResume && (
            <button
              className={inlineActionButton}
              onClick={(e) => { e.stopPropagation(); onResume(); }}
              aria-label={`Resume session ${session.title}`}
            >
              <span aria-hidden="true">▶️</span> Resume
            </button>
          )}
          {isHibernated && onResumeFromHibernation && (
            <button
              className={inlineActionButton}
              onClick={(e) => { e.stopPropagation(); onResumeFromHibernation(); }}
              aria-label={`Wake session ${session.title} from hibernation`}
            >
              <span aria-hidden="true">▶️</span> Resume
            </button>
          )}
          {isRunning && !isCreating && onPause && (
            <button
              className={inlineActionButton}
              onClick={(e) => { e.stopPropagation(); onPause(); }}
              aria-label={`Pause session ${session.title}`}
            >
              <span aria-hidden="true">⏸️</span> Pause
            </button>
          )}
        </span>
        <SessionActionsOverflow
          ref={overflowRef}
          session={session}
          showPrimaryAction={false}
          buttonClassName={rowOverflowButton}
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
          onToggleAutonomousMode={onToggleAutonomousMode}
          onSteerAutonomousSession={onSteerAutonomousSession}
          onClearConversationState={onClearConversationState}
          onUpdateTags={onUpdateTags}
        />
      </span>
    </div>
  );
}
