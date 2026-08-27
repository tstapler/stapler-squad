"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  DndContext,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
  type DragCancelEvent,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { Code } from "@connectrpc/connect";
import { SessionStatus, type Session } from "@/gen/session/v1/types_pb";
import { GroupingStrategy } from "@/lib/grouping/strategies";
import { useFilteredGroupedSessions } from "@/lib/hooks/useFilteredGroupedSessions";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { useApprovalsContext } from "@/lib/contexts/ApprovalsContext";
import { store } from "@/lib/store/store";
import { BOARD_COLUMNS, getBoardColumnKey, type BoardColumnKey } from "@/lib/board/columns";
import { isLegalBoardDragForSession } from "@/lib/board/transitions";
import { statusForColumnMove } from "@/lib/board/statusForColumnMove";
import { parseCompositeId } from "@/lib/board/compositeId";
import type { DragOutcome } from "@/lib/board/dragOutcome";
import { BoardColumn } from "./BoardColumn";
import { BoardCompleteConfirmDialog } from "./BoardCompleteConfirmDialog";
import type { BoardCardProps } from "./BoardCard";
import type { SessionListProps } from "./SessionList";
import { container, board, liveRegion, toastError, toastWarning } from "./SessionBoard.css";

// Identical to SessionList's prop surface so a caller (the future SessionListPaneBody) can
// spread the same props object into either component when switching view modes.
export type SessionBoardProps = SessionListProps;

// Sole swimlane row until Phase 6 (Task 6.1.1b) wires real grouping-strategy row keys — kept
// as a named constant now so the composite id scheme (`${rowKey}:${entityId}`) is uniform
// from Phase 3 onward rather than retrofitted later.
const DEFAULT_ROW_KEY = "__default__";

// Stable empty collections so useFilteredGroupedSessions's memoized pipeline doesn't
// recompute every render purely because a fresh `new Set()`/`new Map()` literal changed
// identity — the board has no delete-undo or cost-sort UI yet, so these stay empty.
const EMPTY_ID_SET = new Set<string>();
const EMPTY_COST_MAP = new Map<string, number>();

// No stale-session config UI on the board yet; matches SessionCard's own internal default.
const STALE_THRESHOLD_MINUTES = 30;

// ConnectRPC codes that indicate a transport-level failure (offline, timeout, server crash)
// rather than a business-rule rejection -- distinguishes DragOutcome's "network_error" from
// "rejected_by_server" per Task 3.2.1c.
const NETWORKISH_ERROR_CODES: number[] = [
  Code.Unavailable,
  Code.DeadlineExceeded,
  Code.Unknown,
  Code.Internal,
];

interface AttemptColumnMoveDeps {
  updateSession: (id: string, updates: { status: SessionStatus }) => Promise<Session | null>;
  resumeHibernatedSession: (id: string) => Promise<Session | null>;
  /** Resolves the session's pending approval (approve). Returns false if none was found/failed. */
  approveNeedsReview: (session: Session) => Promise<boolean>;
  /** Resolves true (Confirm) / false (Cancel) for a drop/move into "Complete" (AC12). */
  confirmComplete: (session: Session) => Promise<boolean>;
  getSessionsErrorState: () => { error: string | null; errorCode?: number };
  /** Called immediately before the mutation fires, so the caller can optimistically re-bucket. */
  onOptimisticMove?: (toColumn: BoardColumnKey) => void;
}

/**
 * Decides and executes one column-move attempt (drag-drop or, from Phase 4, MoveToMenu --
 * both will route through this same function once it's extracted to a shared module). Lives
 * inline in SessionBoard.tsx for now per the plan: extraction to a shared file happens in
 * Phase 4 when MoveToMenu needs the identical logic.
 */
export async function attemptColumnMove(
  session: Session,
  fromColumn: BoardColumnKey,
  toColumn: BoardColumnKey,
  deps: AttemptColumnMoveDeps
): Promise<DragOutcome> {
  // "Needs Review" -> "Running" is a special case: it resolves the pending approval rather
  // than writing a status directly (no SessionStatus corresponds to "leave Needs Review").
  if (fromColumn === "needs_review" && toColumn === "running") {
    const approved = await deps.approveNeedsReview(session);
    if (!approved) {
      return { type: "rejected_by_server", reason: "Failed to resolve the pending approval" };
    }
    return { type: "moved" };
  }

  if (!isLegalBoardDragForSession(session, fromColumn, toColumn)) {
    return { type: "rejected_illegal", from: fromColumn, to: toColumn };
  }

  // AC12: dropping into "Complete" calls StopByUser, which kills the session's tmux pane and
  // can remove its worktree -- legalBoardTransitions["complete"] has no outbound edges, so
  // there's no in-board undo. Require explicit confirmation before the mutation fires.
  if (toColumn === "complete") {
    const confirmed = await deps.confirmComplete(session);
    if (!confirmed) {
      return { type: "cancelled" };
    }
  }

  const status = statusForColumnMove(session, toColumn);

  deps.onOptimisticMove?.(toColumn);

  let result: Session | null;
  if (status === null) {
    if (toColumn === "running" && session.status === SessionStatus.HIBERNATED) {
      result = await deps.resumeHibernatedSession(session.id);
    } else {
      // Unreachable via any drag that passed isLegalBoardDragForSession above -- every legal
      // column pair maps to a concrete status except hibernated->running, handled above.
      return { type: "rejected_illegal", from: fromColumn, to: toColumn };
    }
  } else {
    result = await deps.updateSession(session.id, { status });
  }

  if (!result) {
    const { error, errorCode } = deps.getSessionsErrorState();
    const message = error ?? "Failed to update session";
    const isNetworkish = errorCode !== undefined && NETWORKISH_ERROR_CODES.includes(errorCode);
    return isNetworkish ? { type: "network_error" } : { type: "rejected_by_server", reason: message };
  }

  return { type: "moved" };
}

/**
 * Kanban-style alternative to SessionList: the same sessions, filtered/sorted through the
 * shared pipeline (currently ungrouped -- swimlanes land in a later phase), bucketed into the
 * 4 board columns, with drag-and-drop (dnd-kit) mutating status via `attemptColumnMove`.
 */
export function SessionBoard({
  sessions,
  onSessionClick,
  onSessionOpenInNewPane,
  onDeleteSession,
  onPauseSession,
  onResumeSession,
  onCloneSession,
  onNewWorkspaceSession,
  onRenameSession,
  onRestartSession,
  onUpdateTags,
  onCreateCheckpoint,
  onListCheckpoints,
  onForkFromCheckpoint,
  onRunOneShot,
  onSetRateLimitEnabled,
  onToggleAutonomousMode,
  onToggleAutoApprove,
  onSteerAutonomousSession,
  onClearConversationState,
  onHibernateSession,
  onResumeHibernatedSession,
}: SessionBoardProps) {
  const { updateSession, resumeHibernatedSession } = useSessionService();
  const { approvals, approve } = useApprovalsContext();

  const { filteredSessions } = useFilteredGroupedSessions({
    sessions,
    searchQuery: "",
    selectedStatus: "all",
    selectedCategory: "all",
    selectedTag: "all",
    hidePaused: false,
    showArchived: false,
    filterNeedsApproval: false,
    pendingDeleteIds: EMPTY_ID_SET,
    sortField: "lastActivity",
    sortDir: "desc",
    costById: EMPTY_COST_MAP,
    groupingStrategy: GroupingStrategy.None,
    staleThresholdMinutes: STALE_THRESHOLD_MINUTES,
    staleRecomputeTick: 0,
  });

  const sessionById = useMemo(() => {
    const map = new Map<string, Session>();
    for (const session of sessions) map.set(session.id, session);
    return map;
  }, [sessions]);

  // --- Drag lifecycle state -------------------------------------------------------------
  // inFlightDragSessionIds: session IDs currently mid-drag or mid-mutation -- suppresses
  // watchSessions-driven column reassignment for those IDs until the drag/mutation settles
  // or is cancelled (Story 3.2.2). dragStartColumnRef snapshots each such session's column
  // at pickup time so the frozen render has something to show. optimisticColumnOverrides
  // holds the *destination* column once a legal move starts mutating, for the "moves
  // immediately, pending visual state until the RPC resolves" behavior (Story 3.1.1).
  const [inFlightDragSessionIds, setInFlightDragSessionIds] = useState<ReadonlySet<string>>(new Set());
  const dragStartColumnRef = useRef<Map<string, BoardColumnKey>>(new Map());
  const [optimisticColumnOverrides, setOptimisticColumnOverrides] = useState<Map<string, BoardColumnKey>>(
    new Map()
  );

  // Set right after a "moved" outcome resolves (drag or MoveToMenu); a no-dependency effect
  // below re-checks it on every render and focuses the target once it's back in the DOM
  // (Task 4.2.1b -- focus must land on the card's new location, not fall back to <body>).
  const pendingFocusRef = useRef<{ sessionId: string; via: "drag" | "menu" } | null>(null);

  // Only clears the in-flight/pickup-column tracking -- NOT the optimistic column override.
  // A "moved" outcome must keep its override in place after the RPC settles (see
  // clearOptimisticOverride below for why); only a rejected/cancelled/network-error outcome
  // should call clearOptimisticOverride to snap the card back to its real column.
  const releaseInFlight = useCallback((sessionId: string) => {
    dragStartColumnRef.current.delete(sessionId);
    setInFlightDragSessionIds((prev) => {
      if (!prev.has(sessionId)) return prev;
      const next = new Set(prev);
      next.delete(sessionId);
      return next;
    });
  }, []);

  const clearOptimisticOverride = useCallback((sessionId: string) => {
    setOptimisticColumnOverrides((prev) => {
      if (!prev.has(sessionId)) return prev;
      const next = new Map(prev);
      next.delete(sessionId);
      return next;
    });
  }, []);

  // Shared by both the drag path (handleDragStart) and the MoveToMenu path (handleMenuMove,
  // Task 4.1.1a) so a menu-triggered move freezes watchSessions-driven reassignment exactly
  // the same way a drag does.
  const beginInFlight = useCallback((sessionId: string, fromColumn: BoardColumnKey) => {
    dragStartColumnRef.current.set(sessionId, fromColumn);
    setInFlightDragSessionIds((prev) => new Set(prev).add(sessionId));
  }, []);

  // Once the real `sessions` prop (via a subsequent watchSessions push) reports a session in
  // the column its optimistic override already predicted -- or the session is gone entirely --
  // the override is redundant and is dropped so a later, genuinely different server-driven
  // change isn't incorrectly pinned to the stale override. Deliberately does NOT key off
  // `filteredSessions` (which excludes sessions the current search/filter hides) -- an
  // optimistically-moved session that's about to be filtered out must still reconcile against
  // its real status via `sessionById`, not linger forever because it dropped out of the
  // filtered set.
  useEffect(() => {
    setOptimisticColumnOverrides((prev) => {
      if (prev.size === 0) return prev;
      let changed = false;
      const next = new Map(prev);
      for (const [sessionId, overrideColumn] of prev) {
        const session = sessionById.get(sessionId);
        if (!session || getBoardColumnKey(session) === overrideColumn) {
          next.delete(sessionId);
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [sessionById]);

  const buckets = useMemo(() => {
    const map: Record<BoardColumnKey, Session[]> = {
      running: [],
      needs_review: [],
      paused: [],
      complete: [],
    };
    for (const session of filteredSessions) {
      const override = optimisticColumnOverrides.get(session.id);
      let col: BoardColumnKey;
      if (override) {
        col = override;
      } else if (inFlightDragSessionIds.has(session.id)) {
        col = dragStartColumnRef.current.get(session.id) ?? getBoardColumnKey(session);
      } else {
        col = getBoardColumnKey(session);
      }
      map[col].push(session);
    }
    return map;
  }, [filteredSessions, optimisticColumnOverrides, inFlightDragSessionIds]);

  // --- Toast / announcement surface -----------------------------------------------------
  const [toast, setToast] = useState<{ id: number; message: string; kind: "error" | "warning" } | null>(null);
  const toastIdRef = useRef(0);

  const showToast = useCallback((message: string, kind: "error" | "warning") => {
    toastIdRef.current += 1;
    const id = toastIdRef.current;
    setToast({ id, message, kind });
  }, []);

  useEffect(() => {
    if (!toast) return;
    const timer = setTimeout(() => {
      setToast((current) => (current?.id === toast.id ? null : current));
    }, 4000);
    return () => clearTimeout(timer);
  }, [toast]);

  // --- Screen-reader-only live region (Task 4.2.1a) --------------------------------------
  // Kept independent of `toast` (the visible bubble, error/warning outcomes only) -- a
  // "moved" outcome has no visible toast (the card's new column position is the sighted-user
  // feedback) but still needs its own distinct announcement for screen-reader users.
  const [liveMessage, setLiveMessage] = useState("");
  const announceLive = useCallback((message: string) => setLiveMessage(message), []);

  const columnLabel = useCallback(
    (key?: BoardColumnKey) => BOARD_COLUMNS.find((c) => c.key === key)?.label ?? key ?? "",
    []
  );

  const announceOutcome = useCallback(
    (outcome: DragOutcome, session?: Session, toColumn?: BoardColumnKey) => {
      switch (outcome.type) {
        case "moved":
          announceLive(`${session?.title ?? "Session"} moved to ${columnLabel(toColumn)}.`);
          return;
        case "rejected_illegal": {
          const fromLabel = columnLabel(outcome.from);
          const toLabel = columnLabel(outcome.to);
          const message = `Can't move a ${fromLabel} session to ${toLabel}.`;
          showToast(message, "error");
          announceLive(message);
          return;
        }
        case "rejected_by_server": {
          const message = `"${session?.title ?? "Session"}" already changed state — showing its current status.`;
          showToast(message, "error");
          announceLive(message);
          return;
        }
        case "network_error": {
          const message = `Network error — couldn't move "${session?.title ?? "session"}". Check your connection and try again.`;
          showToast(message, "warning");
          announceLive(message);
          return;
        }
        case "cancelled":
          announceLive("Move cancelled.");
          return;
      }
    },
    [showToast, announceLive, columnLabel]
  );

  // --- Complete-column confirmation (Task 3.1.0 / AC12) ----------------------------------
  const [pendingComplete, setPendingComplete] = useState<{
    session: Session;
    resolve: (confirmed: boolean) => void;
  } | null>(null);

  const confirmComplete = useCallback((session: Session): Promise<boolean> => {
    return new Promise<boolean>((resolve) => {
      setPendingComplete({ session, resolve });
    });
  }, []);

  const handleCompleteConfirm = useCallback(() => {
    pendingComplete?.resolve(true);
    setPendingComplete(null);
  }, [pendingComplete]);

  const handleCompleteCancel = useCallback(() => {
    pendingComplete?.resolve(false);
    setPendingComplete(null);
  }, [pendingComplete]);

  // --- Needs Review resolution (Story 3.3.1) ----------------------------------------------
  const approveNeedsReview = useCallback(
    async (session: Session): Promise<boolean> => {
      const approval = approvals.find((a) => a.sessionId === session.id);
      if (!approval) return false;
      try {
        await approve(approval.id);
        return true;
      } catch {
        return false;
      }
    },
    [approvals, approve]
  );

  const getSessionsErrorState = useCallback(() => {
    const state = store.getState().sessions;
    return { error: state.error, errorCode: state.errorCode };
  }, []);

  // --- dnd-kit wiring ----------------------------------------------------------------------
  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(TouchSensor, { activationConstraint: { delay: 200, tolerance: 8 } })
  );

  const handleDragStart = useCallback(
    (event: DragStartEvent) => {
      const { entityId: sessionId } = parseCompositeId(String(event.active.id));
      const session = sessionById.get(sessionId);
      if (!session) return;
      beginInFlight(sessionId, getBoardColumnKey(session));
      announceLive(`${session.title} picked up.`);
    },
    [sessionById, announceLive, beginInFlight]
  );

  const handleDragCancel = useCallback(
    (event: DragCancelEvent) => {
      const { entityId: sessionId } = parseCompositeId(String(event.active.id));
      releaseInFlight(sessionId);
      announceOutcome({ type: "cancelled" });
    },
    [releaseInFlight, announceOutcome]
  );

  const handleDragEnd = useCallback(
    async (event: DragEndEvent) => {
      const { entityId: sessionId } = parseCompositeId(String(event.active.id));
      const session = sessionById.get(sessionId);
      const fromColumn = dragStartColumnRef.current.get(sessionId);

      if (!session || !fromColumn) {
        releaseInFlight(sessionId);
        return;
      }

      if (!event.over) {
        releaseInFlight(sessionId);
        announceOutcome({ type: "cancelled" });
        return;
      }

      const { entityId: toColumnId } = parseCompositeId(String(event.over.id));
      const toColumn = toColumnId as BoardColumnKey;

      if (fromColumn === toColumn) {
        releaseInFlight(sessionId);
        return;
      }

      const outcome = await attemptColumnMove(session, fromColumn, toColumn, {
        updateSession,
        resumeHibernatedSession,
        approveNeedsReview,
        confirmComplete,
        getSessionsErrorState,
        onOptimisticMove: (target) => {
          setOptimisticColumnOverrides((prev) => new Map(prev).set(sessionId, target));
        },
      });

      releaseInFlight(sessionId);
      if (outcome.type === "moved") {
        // Keep the optimistic override in place -- clearing it here would fall back to the
        // still-stale `sessions` prop (the real watchSessions push hasn't landed yet) and
        // bounce the card back to its old column for one render, which also yanks focus off
        // the element the effect below is about to target. clearOptimisticOverride's own
        // reconciliation effect drops the override once real props catch up.
        pendingFocusRef.current = { sessionId, via: "drag" };
      } else {
        clearOptimisticOverride(sessionId);
      }
      announceOutcome(outcome, session, toColumn);
    },
    [
      sessionById,
      releaseInFlight,
      clearOptimisticOverride,
      announceOutcome,
      updateSession,
      resumeHibernatedSession,
      approveNeedsReview,
      confirmComplete,
      getSessionsErrorState,
    ]
  );

  // MoveToMenu's non-drag counterpart to handleDragEnd: same attemptColumnMove call, same
  // in-flight freeze (Story 3.2.2) so a mid-request watchSessions push can't flicker the card
  // back before the RPC settles, same outcome announcement/focus handling -- only the trigger
  // differs (a discrete menu selection instead of a dnd-kit drop event).
  const attemptMoveViaMenu = useCallback(
    async (session: Session, fromColumn: BoardColumnKey, toColumn: BoardColumnKey) => {
      const sessionId = session.id;
      beginInFlight(sessionId, fromColumn);

      const outcome = await attemptColumnMove(session, fromColumn, toColumn, {
        updateSession,
        resumeHibernatedSession,
        approveNeedsReview,
        confirmComplete,
        getSessionsErrorState,
        onOptimisticMove: (target) => {
          setOptimisticColumnOverrides((prev) => new Map(prev).set(sessionId, target));
        },
      });

      releaseInFlight(sessionId);
      if (outcome.type === "moved") {
        pendingFocusRef.current = { sessionId, via: "menu" };
      } else {
        clearOptimisticOverride(sessionId);
      }
      announceOutcome(outcome, session, toColumn);
      return outcome;
    },
    [
      beginInFlight,
      releaseInFlight,
      clearOptimisticOverride,
      announceOutcome,
      updateSession,
      resumeHibernatedSession,
      approveNeedsReview,
      confirmComplete,
      getSessionsErrorState,
    ]
  );

  // Task 4.2.1b: after a successful move, focus the card's control (drag handle or menu
  // trigger) in its new column rather than letting focus fall back to <body>. No dependency
  // array -- runs after every render, but only acts (and only once) while a focus target is
  // pending, so the cost on unrelated renders is a no-op ref check.
  useEffect(() => {
    const pending = pendingFocusRef.current;
    if (!pending) return;
    const elementId =
      pending.via === "menu"
        ? `board-card-move-trigger-${pending.sessionId}`
        : `board-card-drag-handle-${pending.sessionId}`;
    const el = document.getElementById(elementId);
    if (el) {
      el.focus();
      pendingFocusRef.current = null;
    }
  });

  // Builds the SessionCard-shaped callback props for one session — SessionBoard receives the
  // id/session-keyed handler surface (matching SessionListProps) and adapts it per card here,
  // the same translation SessionList's own card-mode itemContent performs.
  const getCardProps = useCallback(
    (session: Session): Omit<BoardCardProps, "session" | "rowKey"> => ({
      onClick: onSessionClick ? () => onSessionClick(session) : undefined,
      onOpenInNewPane: onSessionOpenInNewPane ? () => onSessionOpenInNewPane(session) : undefined,
      onDelete: onDeleteSession ? () => onDeleteSession(session.id) : undefined,
      onPause: onPauseSession ? () => onPauseSession(session.id) : undefined,
      onResume: onResumeSession ? () => onResumeSession(session) : undefined,
      onClone: onCloneSession ? () => onCloneSession(session.id) : undefined,
      onNewWorkspace: onNewWorkspaceSession ? () => onNewWorkspaceSession(session.id) : undefined,
      onRename: onRenameSession,
      onRestart: onRestartSession,
      onUpdateTags: onUpdateTags,
      onCreateCheckpoint: onCreateCheckpoint,
      onListCheckpoints: onListCheckpoints,
      onForkFromCheckpoint: onForkFromCheckpoint,
      onRunOneShot: onRunOneShot,
      onSetRateLimitEnabled: onSetRateLimitEnabled,
      onToggleAutonomousMode: onToggleAutonomousMode,
      onToggleAutoApprove: onToggleAutoApprove,
      onSteerAutonomousSession: onSteerAutonomousSession,
      onClearConversationState: onClearConversationState,
      onHibernate: onHibernateSession ? () => onHibernateSession(session.id) : undefined,
      onResumeFromHibernation: onResumeHibernatedSession
        ? () => onResumeHibernatedSession(session.id)
        : undefined,
      onMoveToColumn: (toColumn: BoardColumnKey) => {
        void attemptMoveViaMenu(session, getBoardColumnKey(session), toColumn);
      },
    }),
    [
      onSessionClick,
      onSessionOpenInNewPane,
      onDeleteSession,
      onPauseSession,
      onResumeSession,
      onCloneSession,
      onNewWorkspaceSession,
      onRenameSession,
      onRestartSession,
      onUpdateTags,
      onCreateCheckpoint,
      onListCheckpoints,
      onForkFromCheckpoint,
      onRunOneShot,
      onSetRateLimitEnabled,
      onToggleAutonomousMode,
      onToggleAutoApprove,
      onSteerAutonomousSession,
      onClearConversationState,
      attemptMoveViaMenu,
      onHibernateSession,
      onResumeHibernatedSession,
    ]
  );

  return (
    <div className={container} data-context="session-board">
      <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd} onDragCancel={handleDragCancel}>
        <div className={board} role="region" aria-label="Session board">
          {BOARD_COLUMNS.map((col) => (
            <BoardColumn
              key={col.key}
              column={col}
              rowKey={DEFAULT_ROW_KEY}
              sessions={buckets[col.key]}
              getCardProps={getCardProps}
            />
          ))}
        </div>
      </DndContext>

      <div role="status" aria-live="polite" className={liveRegion} data-testid="board-live-region">
        {liveMessage}
      </div>

      {toast && (
        <div
          className={toast.kind === "warning" ? toastWarning : toastError}
          data-testid="board-toast"
        >
          {toast.message}
        </div>
      )}

      {pendingComplete && (
        <BoardCompleteConfirmDialog
          sessionTitle={pendingComplete.session.title}
          onConfirm={handleCompleteConfirm}
          onCancel={handleCompleteCancel}
        />
      )}
    </div>
  );
}
