"use client";
// +feature: remote-host-badge

import { useState, useRef, useEffect, memo } from "react";
import { Session, SessionStatus, SubStatus, ReviewItem, InstanceType, RateLimitState, CheckpointProto, DetectedStatus } from "@/gen/session/v1/types_pb";
import { Tooltip } from "../ui/Tooltip";
import { ReviewQueueBadge } from "./ReviewQueueBadge";
import { RetryBadge } from "./RetryBadge";
import { RevivedContextBadge } from "./RevivedContextBadge";
import { StatusBadge } from "./StatusBadge";
import { SubStatusChip } from "./SubStatusChip";
import { GitHubBadge } from "@/components/shared/GitHubBadge";
import { TagEditor } from "./TagEditor";
import { useTerminalSnapshot } from "@/lib/hooks/useTerminalSnapshot";
import { useSessionActions } from "@/lib/hooks/useSessionActions";
import { useCreationLifecycleActions } from "@/lib/hooks/useCreationLifecycleActions";
import { getFailureMessage } from "@/lib/utils/sessionFailure";
import { DetectionEventsPanel } from "./DetectionEventsPanel";
import { SessionActionsOverflow } from "./SessionActionsOverflow";
import { formatPauseReason } from "@/lib/sessions/formatPauseReason";
import { isAutoApproveSupported } from "@/lib/sessions/autoApprove";
import { getLastActivityTimestamp, isSessionStale } from "@/lib/session-staleness";
import { RemoteConnectionIndicator } from "./RemoteConnectionIndicator";

// The launch command always starts with the program string it was last launched
// with (see Instance.buildLaunchCommand, session/instance_tmux.go). If it no longer
// starts with the current program, the program was changed since the last launch
// and won't take effect until the session is next resumed/restarted. Exported as a
// standalone predicate so it's unit-testable without rendering the full card.
export function hasPendingProgramChange(session: Pick<Session, "status" | "program" | "launchCommand">): boolean {
  const isPausedOrStopped = session.status === SessionStatus.PAUSED || session.status === SessionStatus.STOPPED;
  return (
    isPausedOrStopped &&
    !!session.program &&
    !!session.launchCommand &&
    !session.launchCommand.startsWith(session.program)
  );
}

// A secondary info-row value is redundant with the primary title when it is
// the exact same text (surrounding whitespace aside) — repeating it below the
// title adds visual noise with no new information. Deliberately NOT
// case-insensitive (a user who capitalizes a branch/title differently likely
// meant it) and NOT substring/basename-aware here — callers that need
// basename comparison (Path/Working Dir/Cloned To) pre-normalize via
// `basenameOf` before calling this.
export function isRedundantWithTitle(value: string | undefined | null, title: string): boolean {
  if (!value) return false;
  return value.trim() === title.trim();
}

// Last "/"-separated segment of a trimmed path string. Mirrors the
// `p.split("/").pop() || p` idiom already used in SessionsTable.tsx,
// page.tsx, RecentFilesSection.tsx, and useAvailablePrograms.ts (not
// `path.basename` — no Node `path` polyfill in this "use client" component).
export function basenameOf(pathValue: string): string {
  const trimmed = pathValue.trim();
  return trimmed.split("/").pop() || trimmed;
}

// Path-shaped info rows (Path, Working Dir, Cloned To) compare the value's
// basename against the title, unlike Branch/Goal which compare raw text —
// wrapping that in its own named function keeps the two comparison shapes
// structurally distinct instead of relying on callers to remember which rows
// need basenameOf() and which don't.
function isPathRedundantWithTitle(pathValue: string, title: string): boolean {
  return isRedundantWithTitle(basenameOf(pathValue), title);
}

const AUTO_APPROVE_FLAG_LITERALS = ["--dangerously-skip-permissions", "--yes-always"];

// Mirrors hasPendingProgramChange's shape: true when the persisted autoApprove value
// disagrees with whether a known yolo flag is actually present in the last-launched
// command (i.e. the toggle changed but the process hasn't restarted with it yet).
export function hasPendingAutoApproveChange(session: Pick<Session, "status" | "autoApprove" | "launchCommand">): boolean {
  const isPausedOrStopped = session.status === SessionStatus.PAUSED || session.status === SessionStatus.STOPPED;
  if (!isPausedOrStopped || !session.launchCommand) return false;
  const flagPresent = AUTO_APPROVE_FLAG_LITERALS.some((f) => session.launchCommand.includes(f));
  return session.autoApprove !== flagPresent;
}

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
  hostBadge,
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
  statusCrashed,
  statusCreationFailed,
  statusGlyphIcon,
  failureMessageIcon,
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
  autoApproveBadge,
  autoApprovePendingBadge,
  noteBadge,
  staleBadge,
  creationSpinner,
  actionButton,
  actionButtonCompact,
} from "./SessionCard.css";
import { truncateGoal } from "@/lib/utils/string";

const IS_DEBUG_MODE =
  typeof window !== "undefined" &&
  new URLSearchParams(window.location.search).get("debug") === "1";

// Exported so BoardCard (SessionBoard.tsx's per-card wrapper) can declare an identical
// callback surface without duplicating this list.
export interface SessionCardProps {
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
  onRetryNow?: (sessionId: string) => Promise<boolean>;
  onUpdateTags?: (sessionId: string, tags: string[]) => void;
  onCreateCheckpoint?: (sessionId: string, label: string) => Promise<boolean>;
  onListCheckpoints?: (sessionId: string) => Promise<CheckpointProto[]>;
  onForkFromCheckpoint?: (sessionId: string, checkpointId: string, newTitle: string) => Promise<Session | null>;
  onSetRateLimitEnabled?: (sessionId: string, enabled: boolean) => void;
  onToggleAutonomousMode?: (sessionId: string, enabled: boolean) => void;
  onToggleAutoApprove?: (sessionId: string, enabled: boolean) => void;
  onSteerAutonomousSession?: (sessionId: string, message: string) => Promise<boolean> | void;
  onClearConversationState?: (sessionId: string) => Promise<boolean>;
  onHibernate?: () => void;
  onResumeFromHibernation?: () => void;
  selectMode?: boolean;
  isSelected?: boolean;
  onToggleSelect?: (e?: React.MouseEvent) => void;
  reviewItem?: ReviewItem; // Optional review queue item if session needs attention
  detectedStatus?: DetectedStatus; // Terminal-detected status from pattern analysis
  detectedContext?: string; // Context string for the detected status
  suppressApprovalSubStatus?: boolean; // When true, hides Needs Approval chip/badge during optimistic clear
  // Minutes of inactivity after which an ACTIVE session is flagged "Stale" (see
  // lib/session-staleness.ts's isSessionStale). Optional/defaulted so existing call
  // sites and tests that don't thread it through keep compiling; SessionList passes
  // the resolved value from useStaleSessionConfig().
  staleThresholdMinutes?: number;
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
  onRetryNow,
  onUpdateTags,
  onCreateCheckpoint,
  onListCheckpoints,
  onForkFromCheckpoint,
  onSetRateLimitEnabled,
  onToggleAutonomousMode,
  onToggleAutoApprove,
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
  staleThresholdMinutes = 30,
}: SessionCardProps) {
  const sessionActions = useSessionActions(session.id);
  const [isTagEditorOpen, setIsTagEditorOpen] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [isInlineEditing, setIsInlineEditing] = useState(false);
  const [inlineEditValue, setInlineEditValue] = useState("");
  const [inlineEditError, setInlineEditError] = useState<string | null>(null);
  const inlineSavingRef = useRef(false);
  const keyboardCommitRef = useRef(false);
  const cardRef = useRef<HTMLDivElement>(null);
  const snapshotToggleRef = useRef<HTMLButtonElement>(null);
  const tagEditorTriggerRef = useRef<HTMLElement | null>(null);
  const [isSnapshotOpen, setIsSnapshotOpen] = useState(false);

  // Only fetch snapshot for active sessions (creating/paused/loading sessions have stale output).
  // SessionStatus.ACTIVE covers both ACTIVE and legacy RUNNING (same wire value = 1).
  const isSnapshotEnabled = session.status === SessionStatus.ACTIVE && isSnapshotOpen;
  const isCreating = session.status === SessionStatus.CREATING;
  const isFailed = session.status === SessionStatus.FAILED;
  const isPaused = session.status === SessionStatus.PAUSED;

  // Cancel/Retry guards (Epic 5.4, async-session-creation) -- shared with
  // SessionRow.tsx via useCreationLifecycleActions.
  const { cancelDisabled, retryDisabled, handleCancelCreation, handleRetryCreation } =
    useCreationLifecycleActions(session.id, isCreating);
  const pendingProgramChange = hasPendingProgramChange(session);
  const pendingAutoApproveChange = hasPendingAutoApproveChange(session);
  // Gated on AutoApproveSupported-equivalent so the badge can never claim a session is
  // unguarded when the agent doesn't actually support the injected flag (AC4 / pre-mortem
  // #4) -- e.g. autoApprove=true persisted for a since-unsupported program.
  const autoApproveFlagInjectable = session.autoApprove && isAutoApproveSupported(session.program);
  // AC11, explicit product decision (scoped out, not a silent gap): backlog automation's
  // headless review sessions (session/backlog_review.go, PermissionMode:
  // PermissionModeBypassPermissions) and any auto_yes-driven preset also bypass prompts,
  // without ever setting auto_approve -- but Session.permission_mode is not currently
  // exposed on the proto message (only Instance.PermissionMode, server-side), so badging
  // that population would require new proto+adapter plumbing beyond this feature's scope
  // (plan.md deliberately does not touch session/backlog_review.go). Deferred as a named
  // follow-up rather than silently missed; the badge's contract for now is strictly
  // "auto_approve is true and the agent actually supports the injected flag."
  const showAutoApproveBadge = !pendingAutoApproveChange && autoApproveFlagInjectable;
  const trimmedNote = session.note?.trim();
  const noteTooltip = trimmedNote ? truncateGoal(trimmedNote, 120) : undefined;
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
      case SessionStatus.CRASHED:
        return statusCrashed;
      case SessionStatus.PERMANENTLY_FAILED:
        // Reuses CRASHED's error palette — never the same slot/style as a
        // routine NeedsAttention reason (ReviewQueueBadge), which is the
        // ambiguity this status exists to eliminate (research/ux.md).
        return statusCrashed;
      case SessionStatus.FAILED:
        return statusCreationFailed;
      default:
        return statusUnknown;
    }
  };

  // WCAG 1.4.1 (icon + color, not color alone): CRASHED and FAILED are the
  // only two statuses that could otherwise be mistaken for one another by a
  // colorblind user (both render as a red/amber pill), so both get a distinct
  // glyph. Every other status is unambiguous from its label text alone.
  const getStatusIcon = (sessionStatus: SessionStatus): string | null => {
    switch (sessionStatus) {
      case SessionStatus.CRASHED:
        return "✕";
      case SessionStatus.FAILED:
        return "⚠";
      default:
        return null;
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
      case SessionStatus.CRASHED:
        return "Crashed";
      case SessionStatus.PERMANENTLY_FAILED:
      case SessionStatus.FAILED:
        return "Failed";
      default:
        return "Unknown";
    }
  };

  // Failure-reason categories are set verbatim by the async creation pipeline
  // (server/services/session_creation_pipeline.go, stale_creation_sweeper.go)
  // as session.instance_state.go's FailureReason() -- "GitHubResolutionError",
  // "StartupError", "Stale". Three distinct messages per plan.md Story 5.2.2 /
  // UX research §2/§4 (a stalled creation must never read as a user error).
  // session.failureReason is on the wire (types_pb.ts), so read it directly;
  // falls back to creation_progress, which the pipeline already sets to a
  // detailed message for GitHubResolutionError/StartupError (setPhase calls
  // in session_creation_pipeline.go) before the terminal write -- Stale does
  // not get a fresh setPhase call, so a Stale session without failureReason
  // falls back to the generic message instead of whatever stale progress
  // text it was last showing.
  const failureMessage = isFailed
    ? session.failureReason
      ? getFailureMessage(session.failureReason)
      : session.creationProgress || getFailureMessage("")
    : "";

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
    if ((e.key === "Enter" || e.key === " ") && !(e.target instanceof HTMLInputElement) && !(e.target instanceof HTMLTextAreaElement) && !(e.target instanceof HTMLButtonElement) && !(e.target instanceof HTMLAnchorElement) && !(e.target instanceof HTMLSelectElement)) {
      e.preventDefault();
      if (selectMode && onToggleSelect) {
        onToggleSelect();
      } else if (onClick) {
        onClick();
      }
    } else if (e.key === "F2" && !selectMode) {
      e.preventDefault();
      setInlineEditValue(session.title);
      setInlineEditError(null);
      setIsInlineEditing(true);
    }
  };

  const handleCheckboxClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onToggleSelect?.();
  };

  const handleEditTags = (e: React.MouseEvent) => {
    e.stopPropagation();
    tagEditorTriggerRef.current = e.currentTarget as HTMLElement;
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
    inlineSavingRef.current = true;
    const wasKeyboard = keyboardCommitRef.current;
    keyboardCommitRef.current = false;
    const trimmed = inlineEditValue.trim();
    try {
      if (!trimmed || trimmed === session.title) {
        setIsInlineEditing(false);
        setInlineEditError(null);
        if (wasKeyboard) setTimeout(() => cardRef.current?.focus(), 0);
        return;
      }
      setIsInlineEditing(false);
      const success = await onRename?.(session.id, trimmed);
      if (!success) {
        // Re-open inline edit on failure so the user can correct
        setInlineEditValue(trimmed);
        setInlineEditError("Failed to save — try again");
        setIsInlineEditing(true);
      } else {
        setInlineEditError(null);
        if (wasKeyboard) setTimeout(() => cardRef.current?.focus(), 0);
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
      keyboardCommitRef.current = true;
      handleInlineSave();
    } else if (e.key === "Escape") {
      setInlineEditValue(session.title);
      setInlineEditError(null);
      setIsInlineEditing(false);
      setTimeout(() => cardRef.current?.focus(), 0);
    }
  };

  return (
    <>
      {isTagEditorOpen && onUpdateTags && (
        <TagEditor
          tags={session.tags || []}
          onSave={(newTags) => { onUpdateTags(session.id, newTags); setIsTagEditorOpen(false); }}
          onCancel={() => setIsTagEditorOpen(false)}
          triggerRef={tagEditorTriggerRef}
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
      ref={cardRef}
      data-testid="session-card"
      data-paused={isPaused ? "true" : undefined}
      onClick={handleCardClick}
      onKeyDown={handleCardKeyDown}
      role="group"
      aria-roledescription="session"
      tabIndex={0}
      aria-label={selectMode ? `${isSelected ? "Selected" : "Not selected"}: ${session.title}` : !isInlineEditing ? `${session.title}, press F2 to rename` : session.title}
      aria-keyshortcuts={!selectMode && !isInlineEditing ? "F2" : undefined}
    >
      {selectMode && (
        <div className={checkbox} aria-hidden="true" onClick={handleCheckboxClick}>
          <input
            type="checkbox"
            checked={isSelected}
            tabIndex={-1}
            onClick={(e) => e.stopPropagation()}
            onChange={(e) => { e.stopPropagation(); onToggleSelect?.(); }}
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
                <span id={`inline-error-${session.id}`} role="alert" style={{ color: 'var(--error)', fontSize: '0.75rem', position: 'absolute', top: '100%', left: 0, whiteSpace: 'nowrap', zIndex: 1 }}>
                  {inlineEditError}
                </span>
              )}
            </span>
          ) : (
            <>
              <span
                className={title}
                onClick={handleTitleClick}
                title={selectMode ? undefined : "Click to rename"}
                style={selectMode ? undefined : { cursor: "text" }}
              >
                {session.title}
              </span>
            </>
          )}
          <div className={badges}>
            {isExternal && (
              <span
                className={externalBadge}
                role="img"
                title={`External session from ${sourceTerminal}${muxEnabled ? " (mux-enabled)" : ""}`}
                aria-label={`External session from ${sourceTerminal}${muxEnabled ? ", mux enabled" : ""}`}
              >
                <span aria-hidden="true">🔗</span> {sourceTerminal}
                {muxEnabled && <span className={muxIndicator} aria-hidden="true">✓</span>}
              </span>
            )}
            {session.remoteName && (
              <span
                className={hostBadge}
                role="img"
                title={`Running on ${session.remoteName}`}
                aria-label={`Running on ${session.remoteName}`}
                data-testid="host-badge"
              >
                <span aria-hidden="true">🖥️</span> {session.remoteName}
              </span>
            )}
            {session.remoteName && <RemoteConnectionIndicator remoteName={session.remoteName} />}
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
            {session.retryAttempt > 0 && (
              <RetryBadge
                retryAttempt={session.retryAttempt}
                retryMaxAttempts={session.retryMaxAttempts}
                compact={true}
              />
            )}
            {isPaused && session.pauseReason ? (
              <Tooltip label={formatPauseReason(session.pauseReason)} side="top">
                <span
                  className={`${status} ${getStatusColor(session.status)}`}
                  role="img"
                  aria-label={`Session status: ${getStatusText(session.status)}`}
                  data-testid="status-pill"
                >
                  {getStatusIcon(session.status) && (
                    <span className={statusGlyphIcon} aria-hidden="true">{getStatusIcon(session.status)}</span>
                  )}
                  {getStatusText(session.status)}
                </span>
              </Tooltip>
            ) : session.status === SessionStatus.PERMANENTLY_FAILED ? (
              <span
                className={`${status} ${getStatusColor(session.status)}`}
                role="img"
                aria-label={`Session status: Failed — gave up after ${session.retryMaxAttempts} attempt${session.retryMaxAttempts === 1 ? "" : "s"}`}
              >
                {getStatusText(session.status)}
              </span>
            ) : session.status === SessionStatus.STOPPED && session.creationProgress ? (
              // ponytail: reuses creationProgress — the field is only cleared on a
              // successful start, so a startup/reconnect failure written here (see
              // instance.SetCreationProgress in health.go / connectrpc_websocket.go)
              // survives past the Creating phase and doubles as a "why stopped" reason.
              <Tooltip label={session.creationProgress} side="top">
                <span
                  className={`${status} ${getStatusColor(session.status)}`}
                  role="img"
                  aria-label={`Session status: ${getStatusText(session.status)} — ${session.creationProgress}`}
                  data-testid="status-pill"
                >
                  {getStatusIcon(session.status) && (
                    <span className={statusGlyphIcon} aria-hidden="true">{getStatusIcon(session.status)}</span>
                  )}
                  {getStatusText(session.status)}
                </span>
              </Tooltip>
            ) : (
              <span
                className={`${status} ${getStatusColor(session.status)}`}
                role="img"
                aria-label={`Session status: ${getStatusText(session.status)}`}
                data-testid="status-pill"
              >
                {getStatusIcon(session.status) && (
                  <span className={statusGlyphIcon} aria-hidden="true">{getStatusIcon(session.status)}</span>
                )}
                {getStatusText(session.status)}
              </span>
            )}
            {session.rateLimitState && session.rateLimitState !== RateLimitState.NONE && (
              <span
                className={`${status} ${getRateLimitStateColor(session.rateLimitState)}`}
                role="img"
                aria-label={`Rate limit: ${getRateLimitStateText(session.rateLimitState)}`}
              >
                {getRateLimitStateText(session.rateLimitState)}
              </span>
            )}
            {/* StatusBadge: only shown when SubStatusChip has nothing to display (UNSPECIFIED or suppressed IDLE).
                When the chip is active, it already carries the status info — showing both is duplication. */}
            {detectedStatus !== undefined &&
              !(suppressApprovalSubStatus && (detectedStatus === DetectedStatus.NEEDS_APPROVAL || detectedStatus === DetectedStatus.INPUT_REQUIRED)) &&
              (session.subStatus === SubStatus.UNSPECIFIED || session.subStatus === SubStatus.IDLE) && (
              <StatusBadge detectedStatus={detectedStatus} context={detectedContext} />
            )}
            <RevivedContextBadge session={session} />
            {/* Sub-status chip from the proto sub_status field.
                ACTIVE covers legacy RUNNING (same wire value via allow_alias).
                Cast to number to bypass TS's duplicate-value narrowing for allow_alias enums. */}
            {(session.status as number) === (SessionStatus.ACTIVE as number) &&
              session.subStatus !== SubStatus.UNSPECIFIED &&
              session.subStatus !== SubStatus.IDLE &&
              !(suppressApprovalSubStatus && (session.subStatus === SubStatus.NEEDS_APPROVAL || session.subStatus === SubStatus.INPUT_REQUIRED)) && (
                <SubStatusChip subStatus={session.subStatus} subagentCount={session.subagentCount} />
              )}
            {isSessionStale(session, staleThresholdMinutes) && (
              <span
                role="img"
                aria-label={`Stale — no output for over ${staleThresholdMinutes} minutes`}
                className={`${staleBadge}`}
              >
                🟠 Stale
              </span>
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
                  role="img"
                  title={`Process RSS: ${mb} MB`}
                  aria-label={label}
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
                >
                  {session.autonomousMaxTurns > 0 ? `Auto-pilot ${session.autonomousTurn}/${session.autonomousMaxTurns}` : "Auto-pilot"}
                </button>
              ) : (
                <span
                  className={autonomousBadge}
                  role="img"
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
                role="img"
                style={{ background: "var(--success-bg)", color: "var(--success)" }}
                data-testid="badge-autonomous-done"
                aria-label="Autonomous run completed"
              >
                Done <span aria-hidden="true">✓</span>
              </span>
            )}
            {session.autonomousOutcome === "stuck" && (
              <span
                className={autonomousBadge}
                role="img"
                style={{ background: "var(--warning-bg)", color: "var(--warning)" }}
                data-testid="badge-autonomous-stuck"
                title="Autonomous run stopped — open session to review and give next instruction"
                aria-label="Autonomous run stopped — needs attention"
              >
                Stuck
              </span>
            )}
            {showAutoApproveBadge && (
              // Direct-disable-on-click only for a non-Active session: SetAutoApprove
              // restarts an Active session unconditionally on disable too (AC6), so an
              // Active session's badge is deliberately non-interactive here -- disabling
              // it goes through the overflow menu's confirm dialog (restart notice),
              // not a silent one-click badge action that would restart the session with
              // no warning.
              onToggleAutoApprove && session.status !== SessionStatus.ACTIVE ? (
                <button
                  className={autoApproveBadge}
                  title="Skipping all permission prompts — click to disable"
                  aria-label="Auto-approve enabled — this session skips permission prompts; click to disable"
                  data-testid="badge-auto-approve"
                  onClick={(e) => { e.stopPropagation(); onToggleAutoApprove(session.id, false); }}
                >
                  ⚡ Auto
                </button>
              ) : (
                <span
                  className={autoApproveBadge}
                  role="img"
                  title={
                    session.status === SessionStatus.ACTIVE
                      ? "Skipping all permission prompts — use the ⋯ menu to disable (restarts the session)"
                      : "Skipping all permission prompts"
                  }
                  aria-label="Auto-approve enabled: this session skips permission prompts"
                  data-testid="badge-auto-approve"
                >
                  ⚡ Auto
                </span>
              )
            )}
            {session.workflowId && (
              <span
                className={workflowBadge}
                role="img"
                title={session.workflowName || session.workflowId}
                aria-label={`Workflow: ${session.workflowName || session.workflowId}`}
                data-testid="workflow-badge"
              >
                <span aria-hidden="true">⚙</span> {session.workflowName || "Workflow"}
              </span>
            )}
            {pendingAutoApproveChange && (
              <span
                className={autoApprovePendingBadge}
                role="img"
                data-testid="badge-pending-auto-approve"
                title="Auto-approve setting changed since this session last launched — takes effect on resume/restart"
                aria-label="Auto-approve change pending: takes effect on resume or restart"
              >
                <span aria-hidden="true">⏳</span> Auto-approve pending
              </span>
            )}
            {pendingProgramChange && (
              <span
                className={workflowBadge}
                role="img"
                data-testid="badge-pending-program"
                title="Program was changed since this session last launched — takes effect on resume/restart"
                aria-label="Program change pending: takes effect on resume or restart"
              >
                <span aria-hidden="true">⏳</span> Pending program change
              </span>
            )}
            {noteTooltip && (
              <Tooltip label={noteTooltip}>
                <span
                  className={noteBadge}
                  role="img"
                  aria-label="Has a note"
                  data-testid="badge-has-note"
                >
                  <span aria-hidden="true">📝</span> Note
                </span>
              </Tooltip>
            )}
          </div>
        </div>
        {session.category && (
          <span className={category} aria-label={`Category: ${session.category}`}>{session.category}</span>
        )}
        <div className={tagsContainer}>
          {session.tags && session.tags.length > 0 && (
            <div className={tags} role="list" aria-label="Session tags">
              {session.tags.map((sessionTag) => (
                <span key={sessionTag} className={tag} role="listitem">
                  {sessionTag}
                </span>
              ))}
            </div>
          )}
          <button
            className={editTagsButton}
            onClick={handleEditTags}
            title="Edit tags"
            aria-label={`${session.tags && session.tags.length > 0 ? "Edit" : "Add"} tags for ${session.title}`}
            tabIndex={selectMode ? -1 : undefined}
            aria-hidden={selectMode ? "true" : undefined}
            inert={selectMode || undefined}
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
          const lastActivity = getLastActivityTimestamp(session);
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
          {session.branch && !isRedundantWithTitle(session.branch, session.title) && (
            <div className={infoRow}>
              <span className={label}>Branch:</span>
              <span className={value}>{session.branch}</span>
            </div>
          )}
          {session.path && !isPathRedundantWithTitle(session.path, session.title) && (
            <div className={infoRow}>
              <span className={label}>Path:</span>
              <span className={value} title={session.path}>
                {session.path}
              </span>
            </div>
          )}
          {session.workingDir && !isPathRedundantWithTitle(session.workingDir, session.title) && (
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
                  aria-label={`GitHub repository ${session.githubOwner}/${session.githubRepo}, opens in new tab`}
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
                  aria-label={`Pull request #${session.githubPrNumber} on ${session.githubOwner}/${session.githubRepo}, opens in new tab`}
                >
                  #{session.githubPrNumber}
                </a>
              </span>
            </div>
          )}
          {session.clonedRepoPath && !isPathRedundantWithTitle(session.clonedRepoPath, session.title) && (
            <div className={infoRow}>
              <span className={label}>Cloned To:</span>
              <span className={value} title={session.clonedRepoPath}>
                {session.clonedRepoPath}
              </span>
            </div>
          )}
          {session.goal?.goalText && !isRedundantWithTitle(session.goal.goalText, session.title) && (
            <div className={infoRow}>
              <span className={label}>Goal</span>
              <span className={value}>
                {truncateGoal(session.goal.goalText, 61)}
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
          <div
            className={diffStats}
            role="img"
            aria-label={`Diff: +${session.diffStats.added} additions, -${session.diffStats.removed} deletions`}
          >
            <span className={diffAdded} aria-hidden="true">+{session.diffStats.added}</span>
            <span className={diffRemoved} aria-hidden="true">-{session.diffStats.removed}</span>
          </div>
        )}

        {/*
          Persistent live region for creation progress AND Failed-state
          announcements — the SAME node is reused for both (plan.md Story
          5.2.2 / design/ux.md's "one live region, not two"), never a second
          role="status" element. aria-live flips polite (routine Creating
          progress) -> assertive (a Failed transition is worth interrupting
          for) via attribute mutation on this node, not a remount, so NVDA/
          VoiceOver reliably pick up the change. Always in the DOM (not
          conditionally mounted) so content-only mutations are what triggers
          the announcement.
        */}
        <span
          role="status"
          aria-live={isFailed ? "assertive" : "polite"}
          id={`creation-status-${session.id}`}
          data-testid="creation-live-region"
          style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clipPath: "inset(50%)", whiteSpace: "nowrap" }}
        >
          {isFailed ? failureMessage : isCreating ? (session.creationProgress || "Starting session...") : ""}
        </span>
        {/* Creation progress spinner — only for Creating sessions */}
        {isCreating && (
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "8px", padding: "8px 0", color: "var(--text-secondary)", fontSize: "0.875rem" }}>
            <span style={{ display: "flex", alignItems: "center", gap: "8px" }}>
              <span className={creationSpinner} aria-hidden="true" />
              <span aria-hidden="true" data-testid="session-progress-text">{session.creationProgress || "Starting session..."}</span>
            </span>
            {/*
              Cancel button (Epic 5.4/Story 5.4.1) — clickable immediately,
              not gated on any delay. On success the instance is deleted
              server-side and the card is removed from the list; on a
              lost-race FailedPrecondition the normal stream status update
              (Active/Failed) takes over instead of a Cancelled flash.
            */}
            <button
              type="button"
              onClick={handleCancelCreation}
              disabled={cancelDisabled}
              aria-label="Cancel session creation"
              data-testid="cancel-creation-button"
              className={`${actionButton} ${actionButtonCompact}`}
            >
              Cancel
            </button>
          </div>
        )}
        {/*
          Persistent Failed-state message — the durable, always-visible
          record of why creation failed (design/ux.md Surface 3: the toast in
          Epic 5.3 is transient, this row is not).
        */}
        {isFailed && (
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "8px", padding: "8px 0", color: "var(--text-secondary)", fontSize: "0.875rem" }}>
            <span style={{ display: "flex", alignItems: "center", gap: "8px" }}>
              <span className={failureMessageIcon} aria-hidden="true">⚠</span>
              <span data-testid="failure-message">{failureMessage}</span>
            </span>
            {/*
              Retry button (Epic 5.4/Story 5.4.2) — transitions the SAME
              card in place (Failed -> Creating), never a second card.
              Disabled synchronously on click (isSubmittingRef-style guard,
              see Omnibar.tsx) so a double-click only fires one RPC.
            */}
            <button
              type="button"
              onClick={handleRetryCreation}
              disabled={retryDisabled}
              aria-label="Retry creating session"
              data-testid="retry-creation-button"
              className={`${actionButton} ${actionButtonCompact}`}
            >
              Retry
            </button>
          </div>
        )}

        {/* Terminal snapshot preview — only for active sessions (ACTIVE covers legacy RUNNING) */}
        {session.status === SessionStatus.ACTIVE && (
          <div className={snapshotSection} onClick={(e) => e.stopPropagation()}>
            <button
              ref={snapshotToggleRef}
              className={snapshotToggle}
              onClick={() => setIsSnapshotOpen((prev) => !prev)}
              aria-expanded={isSnapshotOpen}
              aria-label={isSnapshotOpen ? "Collapse terminal preview" : "Expand terminal preview"}
            >
              <span>Terminal Preview</span>
              <span className={snapshotToggleIcon} aria-hidden="true">
                {isSnapshotOpen ? "▲" : "▼"}
              </span>
            </button>
            {isSnapshotOpen && (
              snapshotLoadingState ? (
                <div className={snapshotLoading} role="status">Loading…</div>
              ) : snapshotErrorMsg ? (
                <div className={snapshotError.base} role="alert">
                  Failed to load preview
                </div>
              ) : snapshotIsEmpty ? (
                <div className={snapshotEmpty} role="status">No recent output</div>
              ) : (
                <div
                  className={snapshotPane}
                  // Safe: content is rendered by ansi-to-html with escapeXML enabled,
                  // or escaped manually in the plain-text fallback path.
                  role="region"
                  dangerouslySetInnerHTML={{ __html: snapshotHtml }}
                  aria-label="Terminal output preview"
                  tabIndex={0}
                  onKeyDown={(e) => { if (e.key === "Escape") { e.currentTarget.blur(); snapshotToggleRef.current?.focus(); } }}
                />
              )
            )}
          </div>
        )}
      </div>

      {IS_DEBUG_MODE && (
        <DetectionEventsPanel sessionId={session.id} program={session.program} />
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
          onRetryNow={onRetryNow}
          onClone={onClone}
          onOpenInNewPane={onOpenInNewPane}
          onNewWorkspace={onNewWorkspace}
          onCreateCheckpoint={onCreateCheckpoint}
          onSetRateLimitEnabled={onSetRateLimitEnabled}
          onToggleAutonomousMode={onToggleAutonomousMode}
          onToggleAutoApprove={onToggleAutoApprove}
          onSteerAutonomousSession={onSteerAutonomousSession}
          onClearConversationState={onClearConversationState}
          onUpdateTags={onUpdateTags}
          onChangeProgram={async (_id, program) => {
            const result = await sessionActions.update({ program });
            if (!result) throw new Error("Failed to change program.");
          }}
        />
      </div>
    </div>
    </>
  );
}

export const SessionCard = memo(SessionCardInner);
