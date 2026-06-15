"use client";

import { useState, useRef, memo } from "react";
import { Session, SessionStatus, SubStatus, ReviewItem, InstanceType, RateLimitState, CheckpointProto } from "@/gen/session/v1/types_pb";
import { Tooltip } from "../ui/Tooltip";
import { ReviewQueueBadge } from "./ReviewQueueBadge";
import { StatusBadge } from "./StatusBadge";
import { SubStatusChip } from "./SubStatusChip";
import { GitHubBadge } from "./GitHubBadge";
import { TagEditor } from "./TagEditor";
import { useTerminalSnapshot } from "@/lib/hooks/useTerminalSnapshot";
import { DetectionEventsPanel } from "./DetectionEventsPanel";
import { SessionActionsOverflow } from "./SessionActionsOverflow";
import { formatPauseReason } from "@/lib/sessions/formatPauseReason";
import {
  card,
  cardDeleting,
  cardSelectMode,
  cardSelected,
  cardExternal,
  cardPaused,
  checkbox,
  header,
  titleRow,
  title,
  inlineTitleInput,
  badges,
  externalBadge,
  muxIndicator,
  reviewInfo,
  reviewContext,
  status,
  statusRunning,
  statusReady,
  statusPaused,
  statusPausedDistinct,
  statusLoading,
  statusNeedsApproval,
  statusUnknown,
  category,
  tagsContainer,
  tags,
  tag,
  editTagsButton,
  body,
  info,
  infoRow,
  label,
  value,
  githubLink,
  diffStats,
  diffAdded,
  diffRemoved,
  lastActivityRow,
  lastActivityLabel,
  lastActivityTime,
  footer,
  timestamps,
  timestamp,
  snapshotSection,
  snapshotToggle,
  snapshotToggleIcon,
  snapshotPane,
  snapshotEmpty,
  snapshotLoading,
  snapshotError,
  memoryBadge,
  memoryBadgeWarning,
  memoryBadgeHigh,
  cardMemoryPressure,
  taskFraction,
  autonomousBadge,
  workflowBadge,
} from "./SessionCard.css";
import { truncateGoal } from "@/lib/utils/string";

const IS_DEBUG_MODE =
  typeof window !== "undefined" &&
  new URLSearchParams(window.location.search).get("debug") === "1";

interface SessionCardProps {
  session: Session;
  onClick?: () => void;
  onOpenInNewPane?: () => void;
  onDelete?: () => Promise<void> | void;
  onPause?: () => void;
  onResume?: () => void;
  onClone?: () => void;
  onNewWorkspace?: () => void;
  onRename?: (sessionId: string, newTitle: string) => Promise<boolean>;
  onRestart?: (sessionId: string) => Promise<boolean>;
  onUpdateTags?: (sessionId: string, tags: string[]) => void;
  onCreateCheckpoint?: (sessionId: string, label: string) => Promise<boolean>;
  onListCheckpoints?: (sessionId: string) => Promise<CheckpointProto[]>;
  onForkFromCheckpoint?: (sessionId: string, checkpointId: string, newTitle: string) => Promise<Session | null>;
  onRunOneShot?: (sessionId: string) => Promise<void>;
  onSetRateLimitEnabled?: (sessionId: string, enabled: boolean) => void;
  onToggleAutonomousMode?: (sessionId: string, enabled: boolean) => void;
  onSteerAutonomousSession?: (sessionId: string, message: string) => void;
  onClearConversationState?: (sessionId: string) => Promise<boolean>;
  onHibernate?: () => void;
  onResumeFromHibernation?: () => void;
  selectMode?: boolean;
  isSelected?: boolean;
  onToggleSelect?: () => void;
  reviewItem?: ReviewItem; // Optional review queue item if session needs attention
  detectedStatus?: string; // Terminal-detected status from pattern analysis
  detectedContext?: string; // Context string for the detected status
  suppressApprovalSubStatus?: boolean; // When true, hides Needs Approval chip/badge during optimistic clear
}

function SessionCardInner({
  session,
  onClick,
  onOpenInNewPane,
  onDelete,
  onPause,
  onResume,
  onClone,
  onNewWorkspace,
  onRename,
  onRestart,
  onUpdateTags,
  onCreateCheckpoint,
  onListCheckpoints,
  onForkFromCheckpoint,
  onRunOneShot,
  onSetRateLimitEnabled,
  onToggleAutonomousMode,
  onSteerAutonomousSession,
  onClearConversationState,
  onHibernate,
  onResumeFromHibernation,
  selectMode = false,
  isSelected = false,
  onToggleSelect,
  reviewItem,
  detectedStatus,
  detectedContext,
  suppressApprovalSubStatus = false,
}: SessionCardProps) {
  const [isTagEditorOpen, setIsTagEditorOpen] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [isInlineEditing, setIsInlineEditing] = useState(false);
  const [inlineEditValue, setInlineEditValue] = useState("");
  const [inlineEditError, setInlineEditError] = useState<string | null>(null);
  const inlineSavingRef = useRef(false);
  const [isSnapshotOpen, setIsSnapshotOpen] = useState(false);

  // Only fetch snapshot for active sessions (creating/paused/loading sessions have stale output).
  // SessionStatus.ACTIVE covers both ACTIVE and legacy RUNNING (same wire value = 1).
  const isSnapshotEnabled = session.status === SessionStatus.ACTIVE && isSnapshotOpen;
  const isCreating = session.status === SessionStatus.CREATING;
  const isPaused = session.status === SessionStatus.PAUSED;
  const { html: snapshotHtml, isEmpty: snapshotIsEmpty, loading: snapshotLoadingState, error: snapshotErrorMsg } =
    useTerminalSnapshot(session.id, isSnapshotEnabled);

  const getStatusColor = (sessionStatus: SessionStatus): string => {
    switch (sessionStatus) {
      case SessionStatus.ACTIVE:  // includes RUNNING (same wire value = 1)
        return statusRunning;
      case SessionStatus.READY:
        return statusReady;
      case SessionStatus.PAUSED:
        return statusPausedDistinct;  // distinct from STOPPED/HIBERNATED which use statusPaused
      case SessionStatus.LOADING:
        return statusLoading;
      case SessionStatus.CREATING:
        return statusLoading;
      case SessionStatus.NEEDS_APPROVAL:
        return statusNeedsApproval;
      case SessionStatus.STOPPED:
        return statusPaused;
      case SessionStatus.HIBERNATED:
        return statusPaused;  // no distinct style yet; reuses paused (session is idle/stopped)
      default:
        return statusUnknown;
    }
  };

  const getStatusText = (sessionStatus: SessionStatus): string => {
    switch (sessionStatus) {
      case SessionStatus.ACTIVE:  // includes RUNNING (same wire value = 1)
        return "Active";
      case SessionStatus.READY:
        return "Ready";
      case SessionStatus.PAUSED:
        return "Paused";
      case SessionStatus.LOADING:
        return "Loading";
      case SessionStatus.NEEDS_APPROVAL:
        return "Needs Approval";
      case SessionStatus.CREATING:
        return "Starting…";
      case SessionStatus.STOPPED:
        return "Stopped";
      case SessionStatus.HIBERNATED:
        return "Hibernated";
      default:
        return "Unknown";
    }
  };

  const formatResetTime = (ts?: { seconds: bigint; nanos: number }): string => {
    if (!ts || ts.seconds === BigInt(0)) return "";
    const date = new Date(Number(ts.seconds) * 1000);
    return "until " + date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  };

  const getRateLimitStateText = (state: RateLimitState): string => {
    switch (state) {
      case RateLimitState.NONE:
        return "";
      case RateLimitState.WAITING: {
        const resetStr = formatResetTime(session.rateLimitResetTime);
        return resetStr ? `Rate limited ${resetStr}` : "Rate Limited";
      }
      case RateLimitState.RECOVERING:
        return "Recovering...";
      case RateLimitState.RECOVERED:
        return "Recovered";
      case RateLimitState.FAILED:
        return "Recovery Failed";
      default:
        return "";
    }
  };

  const getRateLimitStateColor = (state: RateLimitState): string => {
    switch (state) {
      case RateLimitState.NONE:
        return "";
      case RateLimitState.WAITING:
        return statusNeedsApproval;
      case RateLimitState.RECOVERING:
        return statusLoading;
      case RateLimitState.RECOVERED:
        return statusReady;
      case RateLimitState.FAILED:
        return statusPaused;
      default:
        return "";
    }
  };

  const formatDate = (ts?: { seconds: bigint; nanos: number }): string => {
    if (!ts) return "N/A";
    const date = new Date(Number(ts.seconds) * 1000);
    return date.toLocaleString();
  };

  const formatTimeAgo = (ts?: { seconds: bigint; nanos: number }): string => {
    if (!ts || ts.seconds === BigInt(0)) return "Never";
    const now = Date.now();
    const date = new Date(Number(ts.seconds) * 1000);
    const seconds = Math.floor((now - date.getTime()) / 1000);

    if (seconds < 60) return `${seconds}s ago`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
    return `${Math.floor(seconds / 86400)}d ago`;
  };

  const isExternal = session.instanceType === InstanceType.EXTERNAL;

  const sourceTerminal = session.externalMetadata?.sourceTerminal || "External";
  const muxEnabled = session.externalMetadata?.muxEnabled || false;

  const handleCardClick = (e: React.MouseEvent) => {
    if (selectMode && onToggleSelect) {
      e.stopPropagation();
      onToggleSelect();
    } else if (e.altKey && onOpenInNewPane) {
      e.stopPropagation();
      onOpenInNewPane();
    } else if (onClick) {
      e.stopPropagation();
      onClick();
    }
  };

  const handleCardKeyDown = (e: React.KeyboardEvent) => {
    // Support keyboard navigation with Enter or Space
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      if (selectMode && onToggleSelect) {
        onToggleSelect();
      } else if (onClick) {
        onClick();
      }
    }
  };

  const handleCheckboxClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (onToggleSelect) {
      onToggleSelect();
    }
  };

  const handleEditTags = (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsTagEditorOpen(true);
  };

  const handleTitleClick = (e: React.MouseEvent) => {
    if (selectMode) return;
    e.stopPropagation();
    setInlineEditValue(session.title);
    setInlineEditError(null);
    setIsInlineEditing(true);
  };

  const handleInlineSave = async () => {
    if (inlineSavingRef.current) return;
    const trimmed = inlineEditValue.trim();
    if (!trimmed || trimmed === session.title) {
      setIsInlineEditing(false);
      setInlineEditError(null);
      return;
    }
    inlineSavingRef.current = true;
    setIsInlineEditing(false);
    try {
      const success = await onRename?.(session.id, trimmed);
      if (!success) {
        // Re-open inline edit on failure so the user can correct
        setInlineEditValue(trimmed);
        setInlineEditError("Failed to save — try again");
        setIsInlineEditing(true);
      } else {
        setInlineEditError(null);
      }
    } catch {
      setInlineEditValue(trimmed);
      setInlineEditError("Failed to save — try again");
      setIsInlineEditing(true);
    } finally {
      inlineSavingRef.current = false;
    }
  };

  const handleInlineKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      handleInlineSave();
    } else if (e.key === "Escape") {
      setIsInlineEditing(false);
    }
  };

  return (
    <>
      {isTagEditorOpen && onUpdateTags && (
        <TagEditor
          tags={session.tags || []}
          onSave={(newTags) => { onUpdateTags(session.id, newTags); setIsTagEditorOpen(false); }}
          onCancel={() => setIsTagEditorOpen(false)}
          sessionTitle={session.title}
        />
      )}
    <div
      className={[
        card,
        selectMode ? cardSelectMode : "",
        isSelected ? cardSelected : "",
        isExternal ? cardExternal : "",
        isDeleting ? cardDeleting : "",
        Number(session.memoryRssMb ?? 0n) > 500 ? cardMemoryPressure : "",
        isPaused ? cardPaused : "",
      ].filter(Boolean).join(" ")}
      data-testid="session-card"
      data-paused={isPaused ? "true" : undefined}
      onClick={handleCardClick}
      onKeyDown={handleCardKeyDown}
      role="group"
      aria-roledescription="session"
      tabIndex={0}
      aria-label={`Session ${session.title}, status: ${getStatusText(session.status)}, program: ${session.program}`}
    >
      {selectMode && (
        <div className={checkbox} onClick={handleCheckboxClick}>
          <input
            type="checkbox"
            checked={isSelected}
            onChange={(e) => { e.stopPropagation(); onToggleSelect?.(); }}
            aria-label={`Select ${session.title}`}
          />
        </div>
      )}
      <div className={header}>
        <div className={titleRow}>
          {isInlineEditing ? (
            <span style={{ position: 'relative', display: 'inline-block' }}>
              <input
                className={inlineTitleInput}
                value={inlineEditValue}
                autoFocus
                onChange={(e) => setInlineEditValue(e.target.value)}
                onBlur={handleInlineSave}
                onKeyDown={handleInlineKeyDown}
                onClick={(e) => e.stopPropagation()}
                aria-label="Edit session title"
                aria-describedby={inlineEditError ? `inline-error-${session.id}` : undefined}
              />
              {inlineEditError && (
                <span id={`inline-error-${session.id}`} style={{ color: 'var(--error)', fontSize: '0.75rem', position: 'absolute', top: '100%', left: 0, whiteSpace: 'nowrap' }}>
                  {inlineEditError}
                </span>
              )}
            </span>
          ) : (
            <h3
              className={title}
              onClick={handleTitleClick}
              title={selectMode ? undefined : "Click to rename"}
              style={selectMode ? undefined : { cursor: "text" }}
            >
              {session.title}
            </h3>
          )}
          <div className={badges}>
            {isExternal && (
              <span
                className={externalBadge}
                title={`External session from ${sourceTerminal}${muxEnabled ? " (mux-enabled)" : ""}`}
                aria-label={`External session from ${sourceTerminal}`}
              >
                🔗 {sourceTerminal}
                {muxEnabled && <span className={muxIndicator}>✓</span>}
              </span>
            )}
            <GitHubBadge
              prNumber={session.githubPrNumber}
              prUrl={session.githubPrUrl}
              owner={session.githubOwner}
              repo={session.githubRepo}
              sourceRef={session.githubSourceRef}
              prPriority={session.githubPrPriority}
              prState={session.githubPrState}
              isDraft={session.githubPrIsDraft}
              approvedCount={session.githubApprovedCount}
              changesRequestedCount={session.githubChangesReqCount}
              checkConclusion={session.githubCheckConclusion}
              compact={true}
            />
            {reviewItem && (
              <ReviewQueueBadge
                priority={reviewItem.priority}
                reason={reviewItem.reason}
                compact={true}
              />
            )}
            {isPaused && session.pauseReason ? (
              <Tooltip label={formatPauseReason(session.pauseReason)} side="top">
                <span
                  className={`${status} ${getStatusColor(session.status)}`}
                  role="status"
                  aria-label={`Session status: ${getStatusText(session.status)}`}
                >
                  {getStatusText(session.status)}
                </span>
              </Tooltip>
            ) : (
              <span
                className={`${status} ${getStatusColor(session.status)}`}
                role="status"
                aria-label={`Session status: ${getStatusText(session.status)}`}
              >
                {getStatusText(session.status)}
              </span>
            )}
            {session.rateLimitState && session.rateLimitState !== RateLimitState.NONE && (
              <span
                className={`${status} ${getRateLimitStateColor(session.rateLimitState)}`}
                role="status"
                aria-label={`Rate limit: ${getRateLimitStateText(session.rateLimitState)}`}
              >
                {getRateLimitStateText(session.rateLimitState)}
              </span>
            )}
            {/* StatusBadge: only shown when SubStatusChip has nothing to display (UNSPECIFIED or suppressed IDLE).
                When the chip is active, it already carries the status info — showing both is duplication. */}
            {detectedStatus &&
              !(suppressApprovalSubStatus && (detectedStatus === "Needs Approval" || detectedStatus === "Input Required")) &&
              (session.subStatus === SubStatus.UNSPECIFIED || session.subStatus === SubStatus.IDLE) && (
              <StatusBadge detectedStatus={detectedStatus} context={detectedContext} />
            )}
            {/* Sub-status chip from the proto sub_status field.
                ACTIVE covers legacy RUNNING (same wire value via allow_alias).
                Cast to number to bypass TS's duplicate-value narrowing for allow_alias enums. */}
            {(session.status as number) === (SessionStatus.ACTIVE as number) &&
              session.subStatus !== SubStatus.UNSPECIFIED &&
              session.subStatus !== SubStatus.IDLE &&
              !(suppressApprovalSubStatus && (session.subStatus === SubStatus.NEEDS_APPROVAL || session.subStatus === SubStatus.INPUT_REQUIRED)) && (
                <SubStatusChip subStatus={session.subStatus} />
              )}
            {(() => {
              const mb = Number(session.memoryRssMb ?? 0n);
              if (mb <= 0) return null;
              const severityClass =
                mb > 500 ? memoryBadgeHigh :
                mb > 300 ? memoryBadgeWarning : "";
              const label =
                mb >= 1024
                  ? `${(mb / 1024).toFixed(1)} GB RAM`
                  : `${mb} MB RAM`;
              return (
                <span
                  className={[memoryBadge, severityClass].filter(Boolean).join(" ")}
                  title={`Process RSS: ${mb} MB`}
                >
                  {label}
                </span>
              );
            })()}
            {session.autonomousMode && (
              onToggleAutonomousMode ? (
                <button
                  className={autonomousBadge}
                  title="Running under LLM orchestration — click to disable"
                  aria-label={`Auto-pilot active${session.autonomousMaxTurns > 0 ? ` (turn ${session.autonomousTurn}/${session.autonomousMaxTurns})` : ""} — click to disable`}
                  data-testid="badge-autonomous"
                  onClick={(e) => { e.stopPropagation(); onToggleAutonomousMode(session.id, false); }}
                  onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onToggleAutonomousMode(session.id, false); } }}
                >
                  {session.autonomousMaxTurns > 0 ? `Auto-pilot ${session.autonomousTurn}/${session.autonomousMaxTurns}` : "Auto-pilot"}
                </button>
              ) : (
                <span
                  className={autonomousBadge}
                  role="status"
                  title="Running under LLM orchestration — injects prompts automatically"
                  aria-label="Autonomous mode: session is controlled by LLM orchestration"
                  data-testid="badge-autonomous"
                >
                  {session.autonomousMaxTurns > 0 ? `Auto-pilot ${session.autonomousTurn}/${session.autonomousMaxTurns}` : "Auto-pilot"}
                </span>
              )
            )}
            {session.autonomousOutcome === "done" && (
              <span
                className={autonomousBadge}
                role="status"
                style={{ background: "var(--success-bg)", color: "var(--success)" }}
                data-testid="badge-autonomous-done"
              >
                Done ✓
              </span>
            )}
            {session.autonomousOutcome === "stuck" && (
              <span
                className={autonomousBadge}
                role="status"
                style={{ background: "var(--warning-bg)", color: "var(--warning)" }}
                data-testid="badge-autonomous-stuck"
                title="Autonomous run stopped — open session to review and give next instruction"
              >
                Stuck
              </span>
            )}
            {session.workflowId && (
              <span
                className={workflowBadge}
                title={session.workflowName || session.workflowId}
                data-testid="workflow-badge"
              >
                ⚙ {session.workflowName || "Workflow"}
              </span>
            )}
          </div>
        </div>
        {session.category && (
          <span className={category}>{session.category}</span>
        )}
        <div className={tagsContainer}>
          {session.tags && session.tags.length > 0 && (
            <div className={tags}>
              {session.tags.map((sessionTag, index) => (
                <span key={index} className={tag}>
                  {sessionTag}
                </span>
              ))}
            </div>
          )}
          <button
            className={editTagsButton}
            onClick={handleEditTags}
            title="Edit tags"
          >
            {session.tags && session.tags.length > 0 ? "Edit Tags" : "Add Tags"}
          </button>
        </div>
        {reviewItem && !selectMode && (
          <div className={reviewInfo}>
            <ReviewQueueBadge
              priority={reviewItem.priority}
              reason={reviewItem.reason}
              compact={false}
            />
            {reviewItem.context && (
              <span className={reviewContext}>{reviewItem.context}</span>
            )}
          </div>
        )}
        {/* Last Activity — Tier 1 always-visible in header */}
        {(() => {
          const moSecs = session.lastMeaningfulOutput?.seconds ?? BigInt(0);
          const tuSecs = session.lastTerminalUpdate?.seconds ?? BigInt(0);
          const lastActivity = moSecs === BigInt(0) && tuSecs === BigInt(0)
            ? undefined
            : moSecs >= tuSecs ? session.lastMeaningfulOutput : session.lastTerminalUpdate;
          return lastActivity ? (
            <div className={lastActivityRow}>
              <span className={lastActivityLabel}>Active</span>
              <time
                dateTime={new Date(Number(lastActivity.seconds) * 1000).toISOString()}
                title={new Date(Number(lastActivity.seconds) * 1000).toISOString()}
                className={lastActivityTime}
              >
                {formatTimeAgo(lastActivity)}
              </time>
            </div>
          ) : null;
        })()}
      </div>

      <div className={body}>
        <div className={info}>
          <div className={infoRow}>
            <span className={label}>Program:</span>
            <span className={value}>{session.program}</span>
          </div>
          {session.branch && (
            <div className={infoRow}>
              <span className={label}>Branch:</span>
              <span className={value}>{session.branch}</span>
            </div>
          )}
          <div className={infoRow}>
            <span className={label}>Path:</span>
            <span className={value} title={session.path}>
              {session.path}
            </span>
          </div>
          {session.workingDir && (
            <div className={infoRow}>
              <span className={label}>Working Dir:</span>
              <span className={value}>{session.workingDir}</span>
            </div>
          )}
          {session.githubOwner && session.githubRepo && (
            <div className={infoRow}>
              <span className={label}>Repository:</span>
              <span className={value}>
                <a
                  href={`https://github.com/${session.githubOwner}/${session.githubRepo}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={(e) => e.stopPropagation()}
                  className={githubLink}
                >
                  {session.githubOwner}/{session.githubRepo}
                </a>
              </span>
            </div>
          )}
          {session.githubPrNumber > 0 && session.githubPrUrl && (
            <div className={infoRow}>
              <span className={label}>Pull Request:</span>
              <span className={value}>
                <a
                  href={session.githubPrUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={(e) => e.stopPropagation()}
                  className={githubLink}
                >
                  #{session.githubPrNumber}
                </a>
              </span>
            </div>
          )}
          {session.clonedRepoPath && (
            <div className={infoRow}>
              <span className={label}>Cloned To:</span>
              <span className={value} title={session.clonedRepoPath}>
                {session.clonedRepoPath}
              </span>
            </div>
          )}
          {session.goal?.goalText && (
            <div className={infoRow}>
              <span className={label}>Goal</span>
              <span className={value}>
                {truncateGoal(session.goal.goalText, 60)}
                {(session.goal.tasksTotal ?? 0) > 0 && (
                  <span className={taskFraction}>
                    {` · ${session.goal.tasksDone}/${session.goal.tasksTotal} done`}
                  </span>
                )}
              </span>
            </div>
          )}
        </div>

        {session.diffStats && (
          <div className={diffStats}>
            <span className={diffAdded}>+{session.diffStats.added}</span>
            <span className={diffRemoved}>-{session.diffStats.removed}</span>
          </div>
        )}

        {/* Creation progress spinner — only for Creating sessions */}
        {isCreating && (
          <div style={{ display: "flex", alignItems: "center", gap: "8px", padding: "8px 0", color: "var(--text-secondary)", fontSize: "0.875rem" }}>
            <span
              role="status"
              aria-label="Session is starting"
              style={{ display: "inline-block", width: "14px", height: "14px", border: "2px solid var(--primary)", borderTopColor: "transparent", borderRadius: "50%", animation: "spin 0.8s linear infinite" }}
            />
            <span>{session.creationProgress || "Starting session..."}</span>
          </div>
        )}

        {/* Terminal snapshot preview — only for active sessions (ACTIVE covers legacy RUNNING) */}
        {session.status === SessionStatus.ACTIVE && (
          <div className={snapshotSection} onClick={(e) => e.stopPropagation()}>
            <button
              className={snapshotToggle}
              onClick={() => setIsSnapshotOpen((prev) => !prev)}
              aria-expanded={isSnapshotOpen}
              aria-label="Toggle terminal preview"
            >
              <span>Terminal Preview</span>
              <span className={snapshotToggleIcon} aria-hidden="true">
                {isSnapshotOpen ? "▲" : "▼"}
              </span>
            </button>
            {isSnapshotOpen && (
              snapshotLoadingState ? (
                <div className={snapshotLoading}>Loading…</div>
              ) : snapshotErrorMsg ? (
                <div className={snapshotError.base}>
                  Failed to load preview
                </div>
              ) : snapshotIsEmpty ? (
                <div className={snapshotEmpty}>No recent output</div>
              ) : (
                <div
                  className={snapshotPane}
                  // Safe: content is rendered by ansi-to-html with escapeXML enabled,
                  // or escaped manually in the plain-text fallback path.
                  dangerouslySetInnerHTML={{ __html: snapshotHtml }}
                  aria-label="Terminal output preview"
                />
              )
            )}
          </div>
        )}
      </div>

      {IS_DEBUG_MODE && (
        <DetectionEventsPanel sessionId={session.id} />
      )}

      <div className={footer}>
        <div className={timestamps}>
          {session.updatedAt && (
            <span
              className={timestamp}
              title={`Created: ${formatDate(session.createdAt)}\nUpdated: ${formatDate(session.updatedAt)}`}
            >
              Updated <time dateTime={new Date(Number(session.updatedAt.seconds) * 1000).toISOString()}>{formatTimeAgo(session.updatedAt)}</time>
            </span>
          )}
        </div>

        <SessionActionsOverflow
          session={session}
          showPrimaryAction
          onResume={onResume}
          onPause={onPause}
          onHibernate={onHibernate}
          onResumeFromHibernation={onResumeFromHibernation}
          onDelete={async () => {
            setIsDeleting(true);
            try { await onDelete?.(); } finally { setIsDeleting(false); }
          }}
          onRestart={onRestart}
          onClone={onClone}
          onOpenInNewPane={onOpenInNewPane}
          onNewWorkspace={onNewWorkspace}
          onCreateCheckpoint={onCreateCheckpoint}
          onRunOneShot={onRunOneShot}
          onSetRateLimitEnabled={onSetRateLimitEnabled}
          onToggleAutonomousMode={onToggleAutonomousMode}
          onSteerAutonomousSession={onSteerAutonomousSession}
          onClearConversationState={onClearConversationState}
          onUpdateTags={onUpdateTags}
        />
      </div>
    </div>
    </>
  );
}

export const SessionCard = memo(SessionCardInner);
