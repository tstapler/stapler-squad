"use client";

import { useState, useRef, useEffect } from "react";
import { createPortal } from "react-dom";
import { Session, SessionStatus, ReviewItem, InstanceType, RateLimitState, CheckpointProto } from "@/gen/session/v1/types_pb";
import { ReviewQueueBadge } from "./ReviewQueueBadge";
import { StatusBadge } from "./StatusBadge";
import { GitHubBadge } from "./GitHubBadge";
import { TagEditor } from "./TagEditor";
import { useTerminalSnapshot } from "@/lib/hooks/useTerminalSnapshot";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import {
  card,
  cardDeleting,
  cardSelectMode,
  cardSelected,
  cardExternal,
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
  desktopActions,
  overflowContainer,
  overflowButton,
  overflowMenu,
  overflowMenuItem,
  overflowMenuItemDanger,
  actions,
  actionsOpen,
  actionsToggle,
  actionButton,
  deleteButton,
  restartButton,
  renameDialog,
  confirmDialog,
  dialogContent,
  warningText,
  renameInput,
  errorMessage,
  dialogActions,
  submitButton,
  cancelButton,
  dangerButton,
  snapshotSection,
  snapshotToggle,
  snapshotToggleIcon,
  snapshotPane,
  snapshotEmpty,
  snapshotLoading,
  snapshotError,
} from "./SessionCard.css";

interface SessionCardProps {
  session: Session;
  onClick?: () => void;
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
  selectMode?: boolean;
  isSelected?: boolean;
  onToggleSelect?: () => void;
  reviewItem?: ReviewItem; // Optional review queue item if session needs attention
  detectedStatus?: string; // Terminal-detected status from pattern analysis
  detectedContext?: string; // Context string for the detected status
}

export function SessionCard({
  session,
  onClick,
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
  selectMode = false,
  isSelected = false,
  onToggleSelect,
  reviewItem,
  detectedStatus,
  detectedContext,
}: SessionCardProps) {
  const [isTagEditorOpen, setIsTagEditorOpen] = useState(false);
  const [showActions, setShowActions] = useState(false);
  const [showOverflow, setShowOverflow] = useState(false);
  const [isRestartConfirmOpen, setIsRestartConfirmOpen] = useState(false);
  const [isDeleteConfirmOpen, setIsDeleteConfirmOpen] = useState(false);
  const [isCheckpointOpen, setIsCheckpointOpen] = useState(false);
  const [checkpointLabel, setCheckpointLabel] = useState("");
  const [isCreatingCheckpoint, setIsCreatingCheckpoint] = useState(false);
  const [isForkOpen, setIsForkOpen] = useState(false);
  const [forkCheckpoints, setForkCheckpoints] = useState<CheckpointProto[]>([]);
  const [forkTitle, setForkTitle] = useState("");
  const [activeForkCheckpointId, setActiveForkCheckpointId] = useState("");
  const [isForking, setIsForking] = useState(false);
  const [isRunningOneShot, setIsRunningOneShot] = useState(false);
  const [oneShotResult, setOneShotResult] = useState<string | null>(null);
  const [isRenaming, setIsRenaming] = useState(false);
  const [isRestarting, setIsRestarting] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [isInlineEditing, setIsInlineEditing] = useState(false);
  const [inlineEditValue, setInlineEditValue] = useState("");
  const [inlineEditError, setInlineEditError] = useState<string | null>(null);
  const inlineSavingRef = useRef(false);
  const [checkpointError, setCheckpointError] = useState("");
  const [isSnapshotOpen, setIsSnapshotOpen] = useState(false);
  const [forkError, setForkError] = useState("");

  // Refs for focus trap: dialog containers and the buttons that trigger them
  const restartDialogRef = useRef<HTMLDivElement>(null);
  const deleteDialogRef = useRef<HTMLDivElement>(null);
  const checkpointDialogRef = useRef<HTMLDivElement>(null);
  const overflowContainerRef = useRef<HTMLDivElement>(null);
  const overflowMenuRef = useRef<HTMLDivElement>(null);
  const restartTriggerRef = useRef<HTMLButtonElement>(null);
  const checkpointTriggerRef = useRef<HTMLButtonElement>(null);

  useFocusTrap(restartDialogRef, isRestartConfirmOpen, restartTriggerRef);
  useFocusTrap(deleteDialogRef, isDeleteConfirmOpen);
  useFocusTrap(checkpointDialogRef, isCheckpointOpen, checkpointTriggerRef);
  useFocusTrap(overflowMenuRef, showOverflow);

  useEffect(() => {
    if (showOverflow && overflowMenuRef.current) {
      const first = overflowMenuRef.current.querySelector<HTMLElement>('[role="menuitem"]');
      first?.focus();
    }
  }, [showOverflow]);

  // Only fetch snapshot for running sessions (paused/loading sessions have stale output)
  const isSnapshotEnabled = session.status === SessionStatus.RUNNING && isSnapshotOpen;
  const { html: snapshotHtml, isEmpty: snapshotIsEmpty, loading: snapshotLoadingState, error: snapshotErrorMsg } =
    useTerminalSnapshot(session.id, isSnapshotEnabled);

  const getStatusColor = (sessionStatus: SessionStatus): string => {
    switch (sessionStatus) {
      case SessionStatus.RUNNING:
        return statusRunning;
      case SessionStatus.READY:
        return statusReady;
      case SessionStatus.PAUSED:
        return statusPaused;
      case SessionStatus.LOADING:
        return statusLoading;
      case SessionStatus.NEEDS_APPROVAL:
        return statusNeedsApproval;
      case SessionStatus.CREATING:
        return statusLoading;
      case SessionStatus.STOPPED:
        return statusPaused;
      default:
        return statusUnknown;
    }
  };

  const getStatusText = (sessionStatus: SessionStatus): string => {
    switch (sessionStatus) {
      case SessionStatus.RUNNING:
        return "Running";
      case SessionStatus.READY:
        return "Ready";
      case SessionStatus.PAUSED:
        return "Paused";
      case SessionStatus.LOADING:
        return "Loading";
      case SessionStatus.NEEDS_APPROVAL:
        return "Needs Approval";
      case SessionStatus.CREATING:
        return "Creating";
      case SessionStatus.STOPPED:
        return "Stopped";
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

  const isPaused = session.status === SessionStatus.PAUSED;
  const isRunning = session.status === SessionStatus.RUNNING;
  const isReady = session.status === SessionStatus.READY;
  const isExternal = session.instanceType === InstanceType.EXTERNAL;

  // Close overflow menu when clicking outside
  useEffect(() => {
    if (!showOverflow) return;
    const handleOutsideClick = (e: MouseEvent) => {
      if (overflowContainerRef.current && !overflowContainerRef.current.contains(e.target as Node)) {
        setShowOverflow(false);
      }
    };
    document.addEventListener("mousedown", handleOutsideClick);
    return () => document.removeEventListener("mousedown", handleOutsideClick);
  }, [showOverflow]);

  const sourceTerminal = session.externalMetadata?.sourceTerminal || "External";
  const muxEnabled = session.externalMetadata?.muxEnabled || false;

  const handleCardClick = (e: React.MouseEvent) => {
    if (selectMode && onToggleSelect) {
      e.stopPropagation();
      onToggleSelect();
    } else if (onClick) {
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

  const handleSaveTags = (newTags: string[]) => {
    if (onUpdateTags) {
      onUpdateTags(session.id, newTags);
    }
    setIsTagEditorOpen(false);
  };

  const handleCancelTagEdit = () => {
    setIsTagEditorOpen(false);
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

  const handleRestartClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsRestartConfirmOpen(true);
  };

  const handleRestartConfirm = async (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsRestarting(true);

    try {
      await onRestart?.(session.id);
      setIsRestartConfirmOpen(false);
    } catch (error) {
      console.error("Failed to restart session:", error);
    } finally {
      setIsRestarting(false);
    }
  };

  const handleRestartCancel = (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsRestartConfirmOpen(false);
  };

  const handleCheckpointClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    setCheckpointLabel("");
    setIsCheckpointOpen(true);
  };

  const handleCheckpointSubmit = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!checkpointLabel.trim()) return;
    setIsCreatingCheckpoint(true);
    setCheckpointError("");
    try {
      const success = await onCreateCheckpoint?.(session.id, checkpointLabel.trim());
      if (success) {
        setIsCheckpointOpen(false);
      } else {
        setCheckpointError("Failed to create checkpoint");
      }
    } catch (error) {
      setCheckpointError(error instanceof Error ? error.message : "Failed to create checkpoint");
    } finally {
      setIsCreatingCheckpoint(false);
    }
  };

  const handleCheckpointCancel = (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsCheckpointOpen(false);
    setCheckpointError("");
  };

  const handleForkClick = async (e: React.MouseEvent) => {
    e.stopPropagation();
    const cps = await onListCheckpoints?.(session.id) ?? [];
    setForkCheckpoints(cps);
    setForkTitle(`${session.title}-fork`);
    setActiveForkCheckpointId(cps.length > 0 ? cps[cps.length - 1].id : "");
    setIsForkOpen(true);
  };

  const handleForkSubmit = async (checkpointId: string) => {
    if (!forkTitle.trim() || !checkpointId) return;
    setIsForking(true);
    setForkError("");
    try {
      const result = await onForkFromCheckpoint?.(session.id, checkpointId, forkTitle.trim());
      if (result) {
        setIsForkOpen(false);
      } else {
        setForkError("Failed to fork session");
      }
    } catch (error) {
      setForkError(error instanceof Error ? error.message : "Failed to fork session");
    } finally {
      setIsForking(false);
    }
  };

  const handleForkCancel = (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsForkOpen(false);
    setForkError("");
  };

  const handleRunOneShot = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!onRunOneShot) return;
    setIsRunningOneShot(true);
    setOneShotResult(null);
    try {
      await onRunOneShot(session.id);
      setOneShotResult("done");
    } catch {
      setOneShotResult("error");
    } finally {
      setIsRunningOneShot(false);
    }
  };


  return (
    <>
      {isTagEditorOpen && (
        <TagEditor
          tags={session.tags || []}
          onSave={handleSaveTags}
          onCancel={handleCancelTagEdit}
          sessionTitle={session.title}
        />
      )}
      {isRestartConfirmOpen && createPortal(
        <div className={confirmDialog} onClick={handleRestartCancel as unknown as React.MouseEventHandler}>
          <div
            ref={restartDialogRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby="restartDialogTitle"
            className={dialogContent}
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => { if (e.key === "Escape") handleRestartCancel(e as unknown as React.MouseEvent); }}
          >
            <h3 id="restartDialogTitle">Restart Session</h3>
            <p>Are you sure you want to restart &quot;{session.title}&quot;?</p>
            <p className={warningText}>This will terminate the current process and start a new one.</p>
            <div className={dialogActions}>
              <button
                onClick={handleRestartConfirm}
                disabled={isRestarting}
                className={dangerButton}
              >
                {isRestarting ? "Restarting..." : "Restart"}
              </button>
              <button
                onClick={handleRestartCancel}
                disabled={isRestarting}
                className={cancelButton}
              >
                Cancel
              </button>
            </div>
          </div>
        </div>,
        document.body
      )}
      {isDeleteConfirmOpen && createPortal(
        <div className={confirmDialog} onClick={() => setIsDeleteConfirmOpen(false)}>
          <div
            ref={deleteDialogRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby="deleteDialogTitle"
            className={dialogContent}
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => { if (e.key === "Escape") setIsDeleteConfirmOpen(false); }}
          >
            <h3 id="deleteDialogTitle">Delete Session</h3>
            <p>Are you sure you want to delete &quot;{session.title}&quot;?</p>
            <p className={warningText}>This action cannot be undone.</p>
            <div className={dialogActions}>
              <button
                onClick={async (e) => {
                  e.stopPropagation();
                  setIsDeleteConfirmOpen(false);
                  setIsDeleting(true);
                  try { await onDelete?.(); } catch { setIsDeleting(false); }
                }}
                disabled={isDeleting}
                className={dangerButton}
              >
                {isDeleting ? "Deleting..." : "Delete"}
              </button>
              <button
                onClick={(e) => { e.stopPropagation(); setIsDeleteConfirmOpen(false); }}
                disabled={isDeleting}
                className={cancelButton}
              >
                Cancel
              </button>
            </div>
          </div>
        </div>,
        document.body
      )}
      {isCheckpointOpen && createPortal(
        <div className={renameDialog} onClick={handleCheckpointCancel as unknown as React.MouseEventHandler}>
          <div
            ref={checkpointDialogRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby="checkpointDialogTitle"
            className={dialogContent}
            onClick={(e) => e.stopPropagation()}
          >
            <h3 id="checkpointDialogTitle">Create Checkpoint</h3>
            <p>Enter a label for this checkpoint of &quot;{session.title}&quot;:</p>
            <input
              type="text"
              value={checkpointLabel}
              onChange={(e) => setCheckpointLabel(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleCheckpointSubmit(e as unknown as React.MouseEvent);
                if (e.key === "Escape") handleCheckpointCancel(e as unknown as React.MouseEvent);
              }}
              placeholder="e.g. before refactor, working state"
              className={renameInput}
              autoFocus
            />
            {checkpointError && <span className={errorMessage}>{checkpointError}</span>}
            <div className={dialogActions}>
              <button
                onClick={handleCheckpointSubmit}
                disabled={isCreatingCheckpoint || !checkpointLabel.trim()}
                className={submitButton}
              >
                {isCreatingCheckpoint ? "Saving..." : "📍 Save Checkpoint"}
              </button>
              <button
                onClick={handleCheckpointCancel}
                disabled={isCreatingCheckpoint}
                className={cancelButton}
              >
                Cancel
              </button>
            </div>
          </div>
        </div>,
        document.body
      )}
    <div
      className={[
        card,
        selectMode ? cardSelectMode : "",
        isSelected ? cardSelected : "",
        isExternal ? cardExternal : "",
        isDeleting ? cardDeleting : "",
      ].filter(Boolean).join(" ")}
      onClick={handleCardClick}
      onKeyDown={handleCardKeyDown}
      role="button"
      tabIndex={0}
      aria-label={`Session ${session.title}, status: ${getStatusText(session.status)}, program: ${session.program}`}
      aria-pressed={selectMode ? isSelected : undefined}
    >
      <div className={checkbox} onClick={handleCheckboxClick}>
        <input
          type="checkbox"
          checked={isSelected}
          onChange={(e) => { e.stopPropagation(); onToggleSelect?.(); }}
          aria-label={`Select ${session.title}`}
        />
      </div>
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
            <span
              className={`${status} ${getStatusColor(session.status)}`}
              role="status"
              aria-label={`Session status: ${getStatusText(session.status)}`}
            >
              {getStatusText(session.status)}
            </span>
            {session.rateLimitState && session.rateLimitState !== RateLimitState.NONE && (
              <span
                className={`${status} ${getRateLimitStateColor(session.rateLimitState)}`}
                role="status"
                aria-label={`Rate limit: ${getRateLimitStateText(session.rateLimitState)}`}
              >
                {getRateLimitStateText(session.rateLimitState)}
              </span>
            )}
            {detectedStatus && (
              <StatusBadge detectedStatus={detectedStatus} context={detectedContext} />
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
          <div className={infoRow}>
            <span className={label}>Branch:</span>
            <span className={value}>{session.branch}</span>
          </div>
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
        </div>

        {session.diffStats && (
          <div className={diffStats}>
            <span className={diffAdded}>+{session.diffStats.added}</span>
            <span className={diffRemoved}>-{session.diffStats.removed}</span>
          </div>
        )}

        {/* Terminal snapshot preview — only for running sessions */}
        {session.status === SessionStatus.RUNNING && (
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

      <div className={footer}>
        <div className={timestamps}>
          <span className={timestamp}>
            Created: <time dateTime={session.createdAt ? new Date(Number(session.createdAt.seconds) * 1000).toISOString() : ""}>{formatDate(session.createdAt)}</time>
          </span>
          <span className={timestamp}>
            Updated: <time dateTime={session.updatedAt ? new Date(Number(session.updatedAt.seconds) * 1000).toISOString() : ""}>{formatDate(session.updatedAt)}</time>
          </span>
          {(() => {
            // Use the most recent of lastMeaningfulOutput and lastTerminalUpdate.
            // lastMeaningfulOutput is gated by a content-signature check, so it can lag
            // behind lastTerminalUpdate when content repeats (e.g. idle prompt).
            const moSecs = session.lastMeaningfulOutput?.seconds ?? BigInt(0);
            const tuSecs = session.lastTerminalUpdate?.seconds ?? BigInt(0);
            const lastActivity = moSecs === BigInt(0) && tuSecs === BigInt(0)
              ? undefined
              : moSecs >= tuSecs ? session.lastMeaningfulOutput : session.lastTerminalUpdate;
            return lastActivity ? (
              <span className={timestamp} title="Last terminal activity">
                Last Activity: <time dateTime={new Date(Number(lastActivity.seconds) * 1000).toISOString()}>{formatTimeAgo(lastActivity)}</time>
              </span>
            ) : null;
          })()}
        </div>

        {/* Desktop: primary action + overflow menu */}
        <div className={desktopActions}>
          {(isPaused || isReady) && (
            <button
              className={actionButton}
              onClick={(e) => { e.stopPropagation(); onResume?.(); }}
              aria-label={`Resume session ${session.title}`}
              title="Resume this session"
            >
              <span aria-hidden="true">▶️</span> Resume
            </button>
          )}
          {isRunning && (
            <button
              className={actionButton}
              onClick={(e) => { e.stopPropagation(); onPause?.(); }}
              aria-label={`Pause session ${session.title}`}
              title="Pause this session"
            >
              <span aria-hidden="true">⏸️</span> Pause
            </button>
          )}
          <div ref={overflowContainerRef} className={overflowContainer}>
            <button
              id={`overflow-btn-${session.id}`}
              className={overflowButton}
              onClick={(e) => { e.stopPropagation(); setShowOverflow((o) => !o); }}
              aria-label="More session actions"
              aria-expanded={showOverflow}
              aria-haspopup="menu"
              aria-controls={`overflow-menu-${session.id}`}
            >
              ···
            </button>
            {showOverflow && (
              <div
                ref={overflowMenuRef}
                id={`overflow-menu-${session.id}`}
                className={overflowMenu}
                role="menu"
                aria-labelledby={`overflow-btn-${session.id}`}
                onClick={(e) => e.stopPropagation()}
                onKeyDown={(e) => { if (e.key === "Escape") setShowOverflow(false); }}
              >
                {!(isPaused || isReady) && (
                  <button
                    role="menuitem"
                    className={overflowMenuItem}
                    onClick={(e) => { e.stopPropagation(); setShowOverflow(false); onResume?.(); }}
                    aria-label={`Resume session ${session.title}`}
                  >
                    <span aria-hidden="true">▶️</span> Resume
                  </button>
                )}
                {!isRunning && (
                  <button
                    role="menuitem"
                    className={overflowMenuItem}
                    onClick={(e) => { e.stopPropagation(); setShowOverflow(false); onPause?.(); }}
                    aria-label={`Pause session ${session.title}`}
                  >
                    <span aria-hidden="true">⏸️</span> Pause
                  </button>
                )}
                <button
                  ref={restartTriggerRef}
                  role="menuitem"
                  className={`${overflowMenuItem} ${overflowMenuItemDanger}`}
                  onClick={(e) => { e.stopPropagation(); setShowOverflow(false); handleRestartClick(e); }}
                  aria-label={`Restart session ${session.title}`}
                >
                  <span aria-hidden="true">🔄</span> Restart
                </button>
                {onCreateCheckpoint && (
                  <button
                    ref={checkpointTriggerRef}
                    role="menuitem"
                    className={overflowMenuItem}
                    onClick={(e) => { e.stopPropagation(); setShowOverflow(false); handleCheckpointClick(e); }}
                    aria-label={`Create checkpoint for session ${session.title}`}
                  >
                    <span aria-hidden="true">📍</span> Checkpoint
                  </button>
                )}
                {onClone && (
                  <button
                    role="menuitem"
                    className={overflowMenuItem}
                    onClick={(e) => { e.stopPropagation(); setShowOverflow(false); onClone(); }}
                    aria-label={`Clone session ${session.title}`}
                  >
                    <span aria-hidden="true">⊕</span> Clone
                  </button>
                )}
                <button
                  role="menuitem"
                  className={overflowMenuItem}
                  onClick={(e) => { e.stopPropagation(); setShowOverflow(false); handleEditTags(e); }}
                  aria-label={`Edit tags for session ${session.title}`}
                >
                  <span aria-hidden="true">🏷️</span> Edit Tags
                </button>
                {onSetRateLimitEnabled && (
                  <button
                    role="menuitem"
                    className={overflowMenuItem}
                    onClick={(e) => { e.stopPropagation(); setShowOverflow(false); onSetRateLimitEnabled(session.id, !session.rateLimitEnabled); }}
                    aria-label={session.rateLimitEnabled ? `Disable auto-resume for ${session.title}` : `Enable auto-resume for ${session.title}`}
                  >
                    <span aria-hidden="true">{session.rateLimitEnabled ? "⏸" : "▶"}</span> {session.rateLimitEnabled ? "Disable auto-resume" : "Enable auto-resume"}
                  </button>
                )}
                <button
                  role="menuitem"
                  className={overflowMenuItem}
                  onClick={(e) => { e.stopPropagation(); setShowOverflow(false); onNewWorkspace?.(); }}
                  aria-label={`New workspace from ${session.title}`}
                >
                  <span aria-hidden="true">➕</span> New Workspace
                </button>
                <button
                  role="menuitem"
                  className={`${overflowMenuItem} ${overflowMenuItemDanger}`}
                  onClick={(e) => { e.stopPropagation(); setShowOverflow(false); setIsDeleteConfirmOpen(true); }}
                  disabled={isDeleting}
                  aria-label={`Delete session ${session.title}`}
                >
                  {isDeleting ? "Deleting..." : <><span aria-hidden="true">🗑️</span> Delete</>}
                </button>
              </div>
            )}
          </div>
        </div>

        {/* Mobile: accordion toggle + full action list */}
        <button
          className={actionsToggle}
          onClick={(e) => { e.stopPropagation(); setShowActions(!showActions); }}
          aria-expanded={showActions}
          aria-controls={`session-actions-${session.id}`}
          aria-label="Toggle session actions"
        >
          Actions {showActions ? "▲" : "▼"}
        </button>
        <div id={`session-actions-${session.id}`} className={`${actions} ${showActions ? actionsOpen : ""}`}>
          {isPaused ? (
            <button
              className={actionButton}
              onClick={(e) => {
                e.stopPropagation();
                onResume?.();
              }}
              aria-label={`Resume session ${session.title}`}
              title="Resume this session"
            >
              <span aria-hidden="true">▶️</span> Resume
            </button>
          ) : (
            <button
              className={actionButton}
              onClick={(e) => {
                e.stopPropagation();
                onPause?.();
              }}
              aria-label={`Pause session ${session.title}`}
              title="Pause this session"
            >
              <span aria-hidden="true">⏸️</span> Pause
            </button>
          )}
          <button
            className={`${actionButton} ${restartButton}`}
            onClick={handleRestartClick}
            title="Restart this session"
            aria-label={`Restart session ${session.title}`}
          >
            <span aria-hidden="true">🔄</span> Restart
          </button>
          {onCreateCheckpoint && (
            <button
              className={actionButton}
              onClick={handleCheckpointClick}
              title="Save a named checkpoint of the current session state"
              aria-label={`Create checkpoint for session ${session.title}`}
            >
              <span aria-hidden="true">📍</span> Checkpoint
            </button>
          )}
          {onClone && (
            <button
              className={actionButton}
              onClick={(e) => { e.stopPropagation(); onClone(); }}
              title="Clone this session (open omnibar pre-filled with this repo)"
              aria-label={`Clone session ${session.title}`}
            >
              <span aria-hidden="true">⊕</span> Clone
            </button>
          )}
          {onRunOneShot && (
            <button
              className={actionButton}
              onClick={handleRunOneShot}
              disabled={isRunningOneShot}
              title="Run claude one-shot to create a PR for this session"
              aria-label={`Create PR for session ${session.title}`}
            >
              {isRunningOneShot ? "Creating PR…" : oneShotResult === "done" ? "✅ PR Created" : oneShotResult === "error" ? "❌ Failed – Retry?" : "🚀 Create PR"}
            </button>
          )}
          <button
            className={actionButton}
            onClick={handleEditTags}
            title="Edit session tags"
            aria-label={`Edit tags for session ${session.title}`}
          >
            <span aria-hidden="true">🏷️</span> Edit Tags
          </button>
          <button
            className={actionButton}
            onClick={(e) => {
              e.stopPropagation();
              onNewWorkspace?.();
            }}
            title="New workspace on the same project (same path, fresh title and branch)"
            aria-label={`New workspace from ${session.title}`}
          >
            <span aria-hidden="true">➕</span> New Workspace
          </button>
          <button
            className={`${actionButton} ${deleteButton}`}
            onClick={(e) => { e.stopPropagation(); setIsDeleteConfirmOpen(true); }}
            disabled={isDeleting}
            aria-label={`Delete session ${session.title}`}
            title="Delete this session"
          >
            {isDeleting ? "Deleting..." : <><span aria-hidden="true">🗑️</span> Delete</>}
          </button>
        </div>
      </div>
    </div>
    </>
  );
}
