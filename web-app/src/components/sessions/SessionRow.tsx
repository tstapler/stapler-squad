"use client";
// +feature: remote-host-badge

import { useRef, memo } from "react";
import { useSessionActions } from "@/lib/hooks/useSessionActions";
import { useCreationLifecycleActions } from "@/lib/hooks/useCreationLifecycleActions";
import { getFailureMessage } from "@/lib/utils/sessionFailure";
import { Session, SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";
import { Tooltip } from "../ui/Tooltip";
import {
  SessionActionsOverflow,
  SessionActionsOverflowHandle,
} from "./SessionActionsOverflow";
import { hasLostContext, RevivedContextBadge } from "./RevivedContextBadge";
import { SubStatusChip } from "./SubStatusChip";
import { GitHubBadge } from "@/components/shared/GitHubBadge";
import { RemoteConnectionIndicator } from "./RemoteConnectionIndicator";
import { isSessionStale } from "@/lib/session-staleness";
import { staleBadge, hostBadge } from "./SessionCard.css";
import {
  row,
  rowPaused,
  rowActive,
  rowSelected,
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
  noteIndicator,
  branchCell,
  rowMemoryPressure,
  checkboxCell,
  checkboxButton,
  failureMessageLine,
} from "./SessionRow.css";
import { failureMessageIcon } from "./SessionCard.css";
import {
  ColumnKey,
  DEFAULT_VISIBLE_COLUMNS,
  buildRowGridTemplate,
} from "./session-columns";
import { truncateGoal } from "@/lib/utils/string";

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
  onRetryNow?: (sessionId: string) => Promise<boolean | void>;
  onCreateCheckpoint?: (sessionId: string, label: string) => Promise<boolean>;
  onSetRateLimitEnabled?: (sessionId: string, enabled: boolean) => void;
  onToggleAutonomousMode?: (sessionId: string, enabled: boolean) => void;
  onToggleAutoApprove?: (sessionId: string, enabled: boolean) => void;
  onSteerAutonomousSession?: (sessionId: string, message: string) => Promise<boolean> | void;
  onClearConversationState?: (sessionId: string) => Promise<boolean>;
  onUpdateTags?: (sessionId: string, tags: string[]) => void;
  onHibernate?: () => void;
  onResumeFromHibernation?: () => void;
  /** When true, hides the Needs Approval SubStatusChip during optimistic clear */
  suppressApprovalSubStatus?: boolean;
  // Minutes of inactivity after which an ACTIVE session is flagged "Stale" (see
  // lib/session-staleness.ts's isSessionStale). Optional/defaulted so existing call
  // sites and tests that don't thread it through keep compiling; SessionList passes
  // the resolved value from useStaleSessionConfig().
  staleThresholdMinutes?: number;
  /** Which optional columns to render. Defaults to DEFAULT_VISIBLE_COLUMNS. */
  visibleColumns?: ColumnKey[];
  /** When true, the checkbox column is interactive and visible on hover/select mode. */
  selectMode?: boolean;
  /** Whether this row is currently in the selected set. */
  isSelected?: boolean;
  /** Called when the checkbox is clicked; receives the native MouseEvent so the parent can inspect e.shiftKey. */
  onToggleSelect?: (e: React.MouseEvent) => void;
}

// Module-level constant avoids repeated BigInt(0) allocations in hot render paths.
const BIGINT_ZERO = BigInt(0);

function getStatusDotValue(status: SessionStatus): string {
  switch (status) {
    case SessionStatus.ACTIVE: // includes legacy RUNNING (same wire value = 1)
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
    case SessionStatus.CRASHED:
      return "crashed";
    case SessionStatus.PERMANENTLY_FAILED:
      return "crashed";
    case SessionStatus.FAILED:
      return "failed";
    default:
      return "idle";
  }
}

const STATUS_DOT_LABELS: Record<string, string> = {
  running: "Running",
  idle: "Idle",
  "paused-session": "Paused",
  paused: "Stopped",
  loading: "Loading",
  "needs-approval": "Needs Approval",
  hibernated: "Hibernated",
  crashed: "Crashed",
  failed: "Failed",
};

function getStatusDotLabel(dotValue: string): string {
  return STATUS_DOT_LABELS[dotValue] ?? dotValue;
}

function formatElapsed(ts?: { seconds: bigint; nanos: number }): string {
  if (!ts || ts.seconds === BIGINT_ZERO) return "";
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

function getLastActivity(
  session: Session,
): { seconds: bigint; nanos: number } | undefined {
  const moSecs = session.lastMeaningfulOutput?.seconds ?? BIGINT_ZERO;
  const tuSecs = session.lastTerminalUpdate?.seconds ?? BIGINT_ZERO;
  if (moSecs === BIGINT_ZERO && tuSecs === BIGINT_ZERO) return undefined;
  return moSecs >= tuSecs
    ? session.lastMeaningfulOutput
    : session.lastTerminalUpdate;
}

function SessionRowInner({
  session,
  onClick,
  onPause,
  onResume,
  onDelete,
  onClone,
  onOpenInNewPane,
  onNewWorkspace,
  onRestart,
  onRetryNow,
  onCreateCheckpoint,
  onSetRateLimitEnabled,
  onToggleAutonomousMode,
  onToggleAutoApprove,
  onSteerAutonomousSession,
  onClearConversationState,
  onUpdateTags,
  onHibernate,
  onResumeFromHibernation,
  suppressApprovalSubStatus = false,
  staleThresholdMinutes = 30,
  visibleColumns = DEFAULT_VISIBLE_COLUMNS,
  selectMode = false,
  isSelected = false,
  onToggleSelect,
}: SessionRowProps) {
  const overflowRef = useRef<SessionActionsOverflowHandle>(null);
  const sessionActions = useSessionActions(session.id);

  const dotStatus = getStatusDotValue(session.status);
  const isPaused = session.status === SessionStatus.PAUSED;
  const isNeedsApproval = session.status === SessionStatus.NEEDS_APPROVAL;
  const isHibernated = session.status === SessionStatus.HIBERNATED;
  const isRunning = session.status === SessionStatus.ACTIVE;
  const isCreating = session.status === SessionStatus.CREATING;
  const isFailed = session.status === SessionStatus.FAILED;

  // Cancel/Retry guards (Epic 5.4, async-session-creation) -- shared with
  // SessionCard.tsx via useCreationLifecycleActions.
  const { cancelDisabled, retryDisabled, handleCancelCreation, handleRetryCreation } =
    useCreationLifecycleActions(session.id, isCreating);
  // Sessions needing user attention always show their primary action --
  // Creating/Failed included so Cancel/Retry are clickable immediately
  // without requiring a hover (Epic 5.4/Story 5.4.1's "clickable
  // immediately when creation starts, not gated on any delay").
  const actionsAlwaysVisible =
    isPaused || isNeedsApproval || isHibernated || isCreating || isFailed;
  // session.failureReason is on the wire (types_pb.ts) -- read it directly.
  // Falls back to creation_progress, which the pipeline already sets to a
  // detailed message for GitHubResolutionError/StartupError before the
  // terminal write; Stale doesn't get a fresh setPhase call, so a Stale
  // session without failureReason falls back to the generic message instead
  // of whatever stale progress text it was last showing (same behavior as
  // SessionCard.tsx, which shares this mapping via lib/utils/sessionFailure).
  const failureMessage = isFailed
    ? session.failureReason
      ? getFailureMessage(session.failureReason)
      : session.creationProgress || getFailureMessage("")
    : "";
  const lastActivity = getLastActivity(session);
  const elapsedText = formatElapsed(lastActivity ?? session.updatedAt);
  // Show branch separately if the branch column is visible; otherwise fold into displayName.
  const showBranchCol = visibleColumns.includes("branch");
  const displayName = showBranchCol
    ? session.title
    : session.branch || session.title;

  const trimmedNote = session.note?.trim();
  const noteTooltip = trimmedNote ? truncateGoal(trimmedNote, 120) : undefined;

  const memMB = Number(session.memoryRssMb ?? 0n);
  const memorySeverityClass =
    memMB > 500 ? memoryBadgeHigh : memMB > 300 ? memoryBadgeWarning : "";

  const handleContextMenu = (e: React.MouseEvent<HTMLDivElement>) => {
    e.preventDefault();
    overflowRef.current?.openAt(e.clientX, e.clientY);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (
      (e.key === "Enter" || e.key === " ") &&
      !(e.target instanceof HTMLButtonElement) &&
      !(e.target instanceof HTMLInputElement) &&
      !(e.target instanceof HTMLTextAreaElement) &&
      !(e.target instanceof HTMLAnchorElement) &&
      !(e.target instanceof HTMLSelectElement)
    ) {
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
        (session.subStatus === SubStatus.PROCESSING ||
          session.subStatus === SubStatus.WAITING_FOR_AGENT)
          ? rowActive
          : "",
        isSelected ? rowSelected : "",
      ]
        .filter(Boolean)
        .join(" ")}
      style={{
        gridTemplateColumns: buildRowGridTemplate(
          visibleColumns ?? DEFAULT_VISIBLE_COLUMNS,
          { reserveCheckbox: true },
        ),
      }}
      data-testid="session-row"
      data-paused={isPaused ? "true" : undefined}
      data-actions-visible={actionsAlwaysVisible ? "true" : undefined}
      onClick={onClick}
      onContextMenu={handleContextMenu}
      onKeyDown={handleKeyDown}
      tabIndex={0}
      aria-label={`Session ${session.title}, status: ${getStatusDotLabel(dotStatus)}, program: ${session.program}${session.path ? `, path: ${abbreviatePath(session.path)}` : ""}${hasLostContext(session) ? ", context: lost" : ""}`}
    >
      {/* Checkbox cell — always in DOM to keep the reserved grid column occupied */}
      <div
        className={checkboxCell}
        aria-hidden={!selectMode ? "true" : undefined}
      >
        <button
          role="checkbox"
          aria-checked={isSelected}
          aria-label={`Select session ${displayName || session.id}`}
          tabIndex={selectMode ? 0 : -1}
          data-testid="session-row-checkbox"
          onClick={(e) => {
            e.stopPropagation();
            onToggleSelect?.(e);
          }}
          className={checkboxButton}
        />
      </div>

      {/* Status dot — always visible */}
      <Tooltip label={`Status: ${getStatusDotLabel(dotStatus)}`}>
        <span
          className={statusDot}
          data-status={dotStatus}
          data-testid={`session-status-${dotStatus}`}
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
              <span
                className={pathStyle}
                role="img"
                aria-label={`Path: ${session.path}`}
              >
                {abbreviatePath(session.path)}
              </span>
            </Tooltip>
          )}
          {session.status === SessionStatus.ACTIVE &&
            session.subStatus !== SubStatus.UNSPECIFIED &&
            session.subStatus !== SubStatus.READY &&
            session.subStatus !== SubStatus.IDLE &&
            !(
              suppressApprovalSubStatus &&
              (session.subStatus === SubStatus.NEEDS_APPROVAL ||
                session.subStatus === SubStatus.INPUT_REQUIRED)
            ) && (
              <SubStatusChip
                subStatus={session.subStatus}
                subagentCount={session.subagentCount}
              />
            )}
          <RevivedContextBadge session={session} />
          {isSessionStale(session, staleThresholdMinutes) && (
            <span
              role="img"
              aria-label={`Stale — no output for over ${staleThresholdMinutes} minutes`}
              className={staleBadge}
            >
              🟠 Stale
            </span>
          )}
          {/* Row-mode counterpart of SessionCard.tsx's host badge (registry entry:
              docs/registry/features/frontend/remote-host-badge.json, lists both files --
              this file's own // +feature: remote-host-badge marker above is a second, duplicate
              marker the frontend scanner will warn-and-skip in favor of whichever file it walks
              first; the per-feature JSON file is hand-maintained and is the actual source of
              truth for which paths are listed, not the scanner's pick -- the JSON's `alsoIn`
              field documenting both files is a new convention this feature introduced, not a
              pre-existing repo pattern; see that file's note for the reasoning). SessionList.tsx's default
              displayMode is "row" (SessionRow, this file), and "card" mode (SessionCard.tsx) is
              reached only via the list header's view toggle, so the badge needs to exist in
              both or most users never see it at all. Added while writing
              remote-workspaces.spec.ts (ssh-remote-workspaces Phase 6 Epic 6.3), which found
              the row-mode gap by actually exercising the default view in a browser. */}
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
          {session.remoteName && (
            <RemoteConnectionIndicator remoteName={session.remoteName} />
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
          {noteTooltip && (
            <Tooltip label={noteTooltip}>
              <span
                className={noteIndicator}
                role="img"
                aria-label="Has a note"
                data-testid="badge-has-note"
              >
                📝
              </span>
            </Tooltip>
          )}
        </span>
        {/*
          Persistent Failed-state message — row-layout equivalent of
          SessionCard.tsx's failure-message row (design/ux.md Surface 3: the
          toast in Epic 5.3 is transient, this line is not). Third line in
          nameCell, under the name and path lines. Retry-button extension
          point (Epic 5.4) is left for that epic, not built here -- same
          scope boundary as SessionCard.tsx's equivalent block.
        */}
        {isFailed && (
          <span className={failureMessageLine}>
            <span className={failureMessageIcon} aria-hidden="true">⚠</span>
            <span data-testid="failure-message">{failureMessage}</span>
          </span>
        )}
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
          role={
            session.diffStats &&
            (session.diffStats.added > 0 || session.diffStats.removed > 0)
              ? "img"
              : undefined
          }
          aria-label={
            session.diffStats &&
            (session.diffStats.added > 0 || session.diffStats.removed > 0)
              ? `Diff: +${session.diffStats.added} -${session.diffStats.removed}`
              : undefined
          }
          aria-hidden={
            session.diffStats &&
            (session.diffStats.added > 0 || session.diffStats.removed > 0)
              ? undefined
              : "true"
          }
        >
          {session.diffStats &&
          (session.diffStats.added > 0 || session.diffStats.removed > 0) ? (
            <>
              <span style={{ color: "var(--success)" }}>
                +{session.diffStats.added}
              </span>{" "}
              <span style={{ color: "var(--error)" }}>
                -{session.diffStats.removed}
              </span>
            </>
          ) : (
            <span style={{ opacity: 0.3 }}>—</span>
          )}
        </span>
      )}

      {/* Memory usage — optional column, colored by severity */}
      {visibleColumns.includes("memory") && (
        <span
          className={[memoryBadge, memorySeverityClass]
            .filter(Boolean)
            .join(" ")}
          role="img"
          title={memMB > 0 ? `Process RSS: ${memMB} MB` : undefined}
          aria-label={memMB > 0 ? `${memMB} MB RAM` : "No memory data"}
        >
          {memMB > 0 ? (
            memMB >= 1024 ? (
              `${(memMB / 1024).toFixed(1)} GB`
            ) : (
              `${memMB} MB`
            )
          ) : (
            <span style={{ opacity: 0.3 }}>—</span>
          )}
        </span>
      )}

      {/* Branch — optional column */}
      {visibleColumns.includes("branch") && (
        <span
          className={branchCell}
          role="img"
          title={session.branch || undefined}
          aria-label={
            session.branch ? `Branch: ${session.branch}` : "No branch"
          }
        >
          {session.branch || <span style={{ opacity: 0.3 }}>—</span>}
        </span>
      )}

      {/* Elapsed time — optional column */}
      {visibleColumns.includes("elapsed") && (
        <time
          className={elapsedStyle}
          dateTime={
            lastActivity
              ? new Date(Number(lastActivity.seconds) * 1000).toISOString()
              : undefined
          }
          title={
            lastActivity
              ? new Date(Number(lastActivity.seconds) * 1000).toLocaleString()
              : undefined
          }
          aria-label={
            elapsedText ? `Last active: ${elapsedText}` : "No recent activity"
          }
        >
          {elapsedText ? (
            <>
              <span className={elapsedIconStyle} aria-hidden="true">
                ⏱
              </span>
              {elapsedText}
            </>
          ) : (
            <span style={{ opacity: 0.3 }}>—</span>
          )}
        </time>
      )}

      {/* Actions: primary (hover-only unless needs attention) + overflow (always visible) */}
      <span className={actionsStyle} role="group" aria-label="Session actions">
        <span className={primaryActionWrapper} role="presentation">
          {(isPaused || isNeedsApproval) && onResume && (
            <button
              className={inlineActionButton}
              onClick={(e) => {
                e.stopPropagation();
                onResume();
              }}
              aria-label={`Resume session ${session.title}`}
            >
              <span aria-hidden="true">▶️</span> Resume
            </button>
          )}
          {isHibernated && onResumeFromHibernation && (
            <button
              className={inlineActionButton}
              onClick={(e) => {
                e.stopPropagation();
                onResumeFromHibernation();
              }}
              aria-label={`Wake session ${session.title} from hibernation`}
            >
              <span aria-hidden="true">▶️</span> Resume
            </button>
          )}
          {isRunning && !isCreating && onPause && (
            <button
              className={inlineActionButton}
              onClick={(e) => {
                e.stopPropagation();
                onPause();
              }}
              aria-label={`Pause session ${session.title}`}
            >
              <span aria-hidden="true">⏸️</span> Pause
            </button>
          )}
          {/*
            Cancel button (Epic 5.4/Story 5.4.1) — clickable immediately,
            not gated on any delay. On success the instance is deleted
            server-side and the row is removed from the list; on a
            lost-race FailedPrecondition the normal stream status update
            takes over instead of a Cancelled flash.
          */}
          {isCreating && (
            <button
              className={inlineActionButton}
              onClick={handleCancelCreation}
              disabled={cancelDisabled}
              aria-label="Cancel session creation"
              data-testid="cancel-creation-button"
            >
              Cancel
            </button>
          )}
          {/*
            Retry button (Epic 5.4/Story 5.4.2) — transitions the SAME row
            in place (Failed -> Creating), never a second row. Disabled
            synchronously on click so a double-click only fires one RPC.
          */}
          {isFailed && (
            <button
              className={inlineActionButton}
              onClick={handleRetryCreation}
              disabled={retryDisabled}
              aria-label="Retry creating session"
              data-testid="retry-creation-button"
            >
              Retry
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
          onRetryNow={onRetryNow}
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
      </span>
      {/*
        Persistent live region for creation progress AND Failed-state
        announcements -- mirrors SessionCard.tsx's single reused role="status"
        span (plan.md Story 5.2.2 / design/ux.md's "one live region, not
        two"). aria-live flips polite (routine Creating progress) ->
        assertive (a Failed transition is worth interrupting for) via
        attribute mutation on this node, not a remount. Always in the DOM
        (not conditionally mounted), visually hidden via the same
        sr-only-style pattern as SessionCard.tsx's version (this file has no
        shared sr-only class today). Absolutely positioned, so it does not
        consume a grid track in this row's CSS grid layout.
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
    </div>
  );
}

export const SessionRow = memo(SessionRowInner);
