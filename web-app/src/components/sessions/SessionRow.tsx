"use client";

import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import { Tooltip } from "../ui/Tooltip";
import {
  row,
  statusDot,
  name as nameStyle,
  agentIcon as agentIconStyle,
  path as pathStyle,
  elapsed as elapsedStyle,
  actions as actionsStyle,
  actionButton,
} from "./SessionRow.css";

interface SessionRowProps {
  session: Session;
  onClick?: () => void;
  onPause?: () => void;
  onResume?: () => void;
  onDelete?: () => Promise<void> | void;
}

function getStatusDotValue(status: SessionStatus): string {
  switch (status) {
    case SessionStatus.RUNNING:
      return "running";
    case SessionStatus.READY:
      return "idle";
    case SessionStatus.PAUSED:
    case SessionStatus.STOPPED:
      return "paused";
    case SessionStatus.LOADING:
    case SessionStatus.CREATING:
      return "loading";
    case SessionStatus.NEEDS_APPROVAL:
      return "needs-approval";
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
  return "◇";
}

function getLastActivity(session: Session): { seconds: bigint; nanos: number } | undefined {
  const moSecs = session.lastMeaningfulOutput?.seconds ?? BigInt(0);
  const tuSecs = session.lastTerminalUpdate?.seconds ?? BigInt(0);
  if (moSecs === BigInt(0) && tuSecs === BigInt(0)) return undefined;
  return moSecs >= tuSecs ? session.lastMeaningfulOutput : session.lastTerminalUpdate;
}

export function SessionRow({ session, onClick, onPause, onResume, onDelete }: SessionRowProps) {
  const dotStatus = getStatusDotValue(session.status);
  const isRunning = session.status === SessionStatus.RUNNING;
  const isPaused = session.status === SessionStatus.PAUSED || session.status === SessionStatus.STOPPED;
  const lastActivity = getLastActivity(session);
  const elapsedText = formatElapsed(lastActivity ?? session.updatedAt);
  const displayName = session.branch || session.title;

  const handleKeyDown = (e: React.KeyboardEvent<HTMLLIElement>) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onClick?.();
    }
  };

  const handlePauseClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onPause?.();
  };

  const handleResumeClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onResume?.();
  };

  const handleDeleteClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onDelete?.();
  };

  return (
    <li
      className={row}
      data-testid="session-row"
      onClick={onClick}
      onKeyDown={handleKeyDown}
      tabIndex={0}
      aria-label={`Session ${session.title}, status: ${dotStatus}, program: ${session.program}`}
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

      {/* Branch / name */}
      <span
        className={nameStyle}
        aria-label={displayName}
        title={displayName}
      >
        {displayName}
      </span>

      {/* Agent icon */}
      <span
        className={agentIconStyle}
        title={session.program}
        aria-label={`Agent: ${session.program}`}
      >
        {getAgentEmoji(session.program)}
      </span>

      {/* Path — tooltip shows full path when column is too narrow */}
      <Tooltip label={session.path} side="bottom">
        <span
          className={pathStyle}
          dir="ltr"
          aria-label={`Path: ${session.path}`}
        >
          {session.path}
        </span>
      </Tooltip>

      {/* Elapsed time */}
      <time
        className={elapsedStyle}
        dateTime={lastActivity ? new Date(Number(lastActivity.seconds) * 1000).toISOString() : undefined}
        title={lastActivity ? new Date(Number(lastActivity.seconds) * 1000).toLocaleString() : undefined}
      >
        {elapsedText}
      </time>

      {/* Hover actions */}
      <span className={actionsStyle} aria-label="Session actions">
        {isRunning && onPause && (
          <Tooltip label="Pause session">
            <button
              className={actionButton}
              onClick={handlePauseClick}
              aria-label={`Pause ${session.title}`}
            >
              ⏸
            </button>
          </Tooltip>
        )}
        {isPaused && onResume && (
          <Tooltip label="Resume session">
            <button
              className={actionButton}
              onClick={handleResumeClick}
              aria-label={`Resume ${session.title}`}
            >
              ▶
            </button>
          </Tooltip>
        )}
        {onDelete && (
          <Tooltip label="Delete session">
            <button
              className={actionButton}
              onClick={handleDeleteClick}
              aria-label={`Delete ${session.title}`}
            >
              ✕
            </button>
          </Tooltip>
        )}
      </span>
    </li>
  );
}
