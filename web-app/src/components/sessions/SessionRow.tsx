"use client";

import { useRef } from "react";
import { Session, SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";
import { Tooltip } from "../ui/Tooltip";
import { SessionActionsOverflow, SessionActionsOverflowHandle } from "./SessionActionsOverflow";
import { SubStatusChip } from "./SubStatusChip";
import {
  row,
  rowPaused,
  statusDot,
  nameCell as nameCellStyle,
  name as nameStyle,
  agentIcon as agentIconStyle,
  path as pathStyle,
  elapsed as elapsedStyle,
  actions as actionsStyle,
  memoryBadge,
  rowMemoryPressure,
} from "./SessionRow.css";

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
}: SessionRowProps) {
  const overflowRef = useRef<SessionActionsOverflowHandle>(null);

  const dotStatus = getStatusDotValue(session.status);
  const isPaused = session.status === SessionStatus.PAUSED;
  const lastActivity = getLastActivity(session);
  const elapsedText = formatElapsed(lastActivity ?? session.updatedAt);
  const displayName = session.branch || session.title;

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
        Number(session.estimatedSavingsMb ?? 0n) > 0 ? rowMemoryPressure : "",
        isPaused ? rowPaused : "",
      ].filter(Boolean).join(" ")}
      data-testid="session-row"
      data-paused={isPaused ? "true" : undefined}
      onClick={onClick}
      onContextMenu={handleContextMenu}
      onKeyDown={handleKeyDown}
      tabIndex={0}
      aria-label={`Session ${session.title}, status: ${isPaused ? "paused" : dotStatus}, program: ${session.program}`}
    >
      {/* Status dot */}
      <Tooltip label={`Status: ${dotStatus}`}>
        <span
          className={statusDot}
          data-status={dotStatus}
          role="img"
          aria-label={`Status: ${dotStatus}`}
        />
      </Tooltip>

      {/* Name + path stacked — name always visible, path wraps below */}
      <span className={nameCellStyle}>
        <span style={{ display: "flex", alignItems: "center", gap: "6px", minWidth: 0 }}>
          <span className={nameStyle} aria-label={displayName} title={displayName}>
            {displayName}
          </span>
          {/* Sub-status chip — only for Active sessions (ACTIVE covers legacy RUNNING) */}
          {session.status === SessionStatus.ACTIVE &&
            session.subStatus !== SubStatus.UNSPECIFIED &&
            session.subStatus !== SubStatus.IDLE && (
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

      {/* Agent icon */}
      <span
        className={agentIconStyle}
        title={session.program}
        aria-label={`Agent: ${session.program}`}
      >
        {getAgentEmoji(session.program)}
      </span>

      {/* Memory usage badge */}
      {Number(session.memoryRssMb ?? 0n) > 0 && (
        <span className={memoryBadge} aria-label={`${Number(session.memoryRssMb)} MB`}>
          {Number(session.memoryRssMb)} MB
        </span>
      )}

      {/* Elapsed time */}
      <time
        className={elapsedStyle}
        dateTime={lastActivity ? new Date(Number(lastActivity.seconds) * 1000).toISOString() : undefined}
        title={lastActivity ? new Date(Number(lastActivity.seconds) * 1000).toLocaleString() : undefined}
      >
        {elapsedText}
      </time>

      {/* Actions — overflow menu with pause/resume shortcut and confirmed delete */}
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
