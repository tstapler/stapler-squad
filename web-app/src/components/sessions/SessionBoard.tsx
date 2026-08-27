"use client";
// +feature: session-board-view

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
import { GroupingStrategy, GroupingStrategyLabels } from "@/lib/grouping/strategies";
import { useFilteredGroupedSessions } from "@/lib/hooks/useFilteredGroupedSessions";
import { usePersistedViewState, type PersistedFieldsConfig } from "@/lib/hooks/usePersistedViewState";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { useApprovalsContext } from "@/lib/contexts/ApprovalsContext";
import { store } from "@/lib/store/store";
import { BOARD_COLUMNS, getBoardColumnKey, type BoardColumnKey } from "@/lib/board/columns";
import { isLegalBoardDragForSession } from "@/lib/board/transitions";
import { statusForColumnMove } from "@/lib/board/statusForColumnMove";
import { parseCompositeId } from "@/lib/board/compositeId";
import type { DragOutcome } from "@/lib/board/dragOutcome";
import { BoardColumn } from "./BoardColumn";
import { BoardSwimlane } from "./BoardSwimlane";
import { BoardCompleteConfirmDialog } from "./BoardCompleteConfirmDialog";
import { BulkActions } from "./BulkActions";
import { TagEditor } from "./TagEditor";
import type { BoardCardProps } from "./BoardCard";
import type { SessionListProps } from "./SessionList";
import { searchInput, select, selectModeButton, selectModeButtonActive } from "./SessionList.css";
import {
  container,
  board,
  boardRows,
  boardHeader,
  boardHeaderSearch,
  liveRegion,
  toastError,
  toastWarning,
} from "./SessionBoard.css";

// Identical to SessionList's prop surface so a caller (the future SessionListPaneBody) can
// spread the same props object into either component when switching view modes.
export type SessionBoardProps = SessionListProps;

// Sole swimlane row when GroupingStrategy.None is active -- kept as a named constant so the
// composite id scheme (`${rowKey}:${entityId}`) is uniform whether or not swimlanes render.
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

const GROUPING_STRATEGY_VALUES = Object.values(GroupingStrategy);

interface BoardViewState {
  searchQuery: string;
  groupingStrategy: GroupingStrategy;
}

// Same localStorage KEY NAMES SessionList.tsx's BASE_STORAGE_KEYS uses for these two fields
// (intentionally duplicated, not imported -- SessionList doesn't export them) so that, per
// Phase 5's "toggling List/Board preserves search/grouping, no reset" AC, both views read and
// write the exact same persisted value once the user has touched either one's control.
const BOARD_SEARCH_QUERY_KEY = "stapler-squad-search-query";
const BOARD_GROUPING_STRATEGY_KEY = "stapler-squad-grouping-strategy";

function buildBoardViewFields(prefix = ""): PersistedFieldsConfig<BoardViewState> {
  return {
    searchQuery: { key: `${prefix}${BOARD_SEARCH_QUERY_KEY}`, defaultValue: "" },
    groupingStrategy: {
      key: `${prefix}${BOARD_GROUPING_STRATEGY_KEY}`,
      // Deliberately GroupingStrategy.None (flat board), NOT SessionList's own Category
      // default -- since the storage key is shared, this fallback only governs the very
      // first time a workspace visits *either* view before anyone has touched the selector.
      defaultValue: GroupingStrategy.None,
      isValid: (v) => GROUPING_STRATEGY_VALUES.includes(v as GroupingStrategy),
    },
  };
}

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
 * Decides and executes one column-move attempt. Shared by both call sites (Task 4.1.1a):
 * `handleDragEnd` calls it directly (once per selected session for a multi-select fan-out,
 * Task 6.3.1c), and `attemptMoveViaMenu` wraps it for MoveToMenu -- so a drag and an
 * equivalent menu selection always converge on identical legality checks, confirmation
 * prompts, and DragOutcome results.
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
 * Kanban-style alternative to SessionList: the same sessions, filtered/sorted/grouped through
 * the shared pipeline, bucketed into the 4 board columns (crossed with swimlane rows when a
 * grouping strategy is active, Task 6.1.1a), with drag-and-drop (dnd-kit) mutating status via
 * `attemptColumnMove`.
 */
export function SessionBoard({
  sessions,
  onSessionClick,
  onSessionOpenInNewPane,
  onDeleteSession,
  onPauseSession,
  onResumeSession,
  onDirectResumeSession,
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
  storageKeyPrefix,
}: SessionBoardProps) {
  const { updateSession, resumeHibernatedSession } = useSessionService();
  const { approvals, approve } = useApprovalsContext();

  // Search + swimlane grouping-strategy state (Task 6.1.1a, 6.2.1a). Persisted under the same
  // localStorage keys SessionList.tsx uses (namespaced by the same `storageKeyPrefix`), so
  // switching List<->Board carries the value over rather than resetting it (Phase 5 AC).
  const boardViewFields = useMemo(() => buildBoardViewFields(storageKeyPrefix), [storageKeyPrefix]);
  const { state: boardViewState, setters: boardViewSetters } = usePersistedViewState<BoardViewState>(boardViewFields);
  const { searchQuery, groupingStrategy } = boardViewState;
  const { searchQuery: setSearchQuery, groupingStrategy: setGroupingStrategy } = boardViewSetters;
  const groupingActive = groupingStrategy !== GroupingStrategy.None;

  const { filteredSessions, groupedSessions, filteredSessionIds } = useFilteredGroupedSessions({
    sessions,
    searchQuery,
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
    groupingStrategy,
    staleThresholdMinutes: STALE_THRESHOLD_MINUTES,
    staleRecomputeTick: 0,
  });

  const sessionById = useMemo(() => {
    const map = new Map<string, Session>();
    for (const session of sessions) map.set(session.id, session);
    return map;
  }, [sessions]);

  // --- Cross-column bulk-select state (Task 6.3.1a) --------------------------------------
  // Mirrors SessionList.tsx's selectMode/selectedSessions shape and semantics exactly
  // (including intersecting against filteredSessionIds so a selection survives filter/search
  // changes without silently including sessions the user can no longer see).
  const [selectMode, setSelectMode] = useState(false);
  const [selectedSessions, setSelectedSessions] = useState<Set<string>>(new Set());
  const [isBulkTagEditing, setIsBulkTagEditing] = useState(false);
  const bulkTagEditorTriggerRef = useRef<HTMLElement | null>(null);

  const activeSelection = useMemo(
    () => new Set([...selectedSessions].filter((id) => filteredSessionIds.has(id))),
    [selectedSessions, filteredSessionIds]
  );

  const handleToggleSelectMode = useCallback(() => {
    setSelectMode((prev) => {
      if (prev) setSelectedSessions(new Set());
      return !prev;
    });
  }, []);

  const handleToggleSession = useCallback((sessionId: string) => {
    setSelectMode(true);
    setSelectedSessions((prev) => {
      const next = new Set(prev);
      if (next.has(sessionId)) {
        next.delete(sessionId);
      } else {
        next.add(sessionId);
      }
      return next;
    });
  }, []);

  const handleSelectAll = useCallback(() => {
    setSelectedSessions(new Set(filteredSessions.map((s) => s.id)));
  }, [filteredSessions]);

  const handleClearSelection = useCallback(() => {
    setSelectedSessions(new Set());
    setSelectMode(false);
  }, []);

  const handlePauseAll = useCallback(() => {
    if (!onPauseSession) return;
    activeSelection.forEach((id) => onPauseSession(id));
    setSelectedSessions(new Set());
    setSelectMode(false);
  }, [onPauseSession, activeSelection]);

  const handleResumeAll = useCallback(() => {
    if (!onDirectResumeSession && !onResumeSession) return;
    activeSelection.forEach((id) => {
      const session = sessionById.get(id);
      if (!session) return;
      if (onDirectResumeSession) {
        onDirectResumeSession(session);
      } else {
        onResumeSession?.(session);
      }
    });
    setSelectedSessions(new Set());
    setSelectMode(false);
  }, [onDirectResumeSession, onResumeSession, activeSelection, sessionById]);

  const handleDeleteAll = useCallback(() => {
    if (!onDeleteSession) return;
    activeSelection.forEach((id) => {
      void onDeleteSession(id);
    });
    setSelectedSessions(new Set());
    setSelectMode(false);
  }, [onDeleteSession, activeSelection]);

  const handleBulkAddTag = useCallback((triggerEl: HTMLElement) => {
    bulkTagEditorTriggerRef.current = triggerEl;
    setIsBulkTagEditing(true);
  }, []);

  const handleBulkTagSave = useCallback(
    (newTags: string[]) => {
      if (newTags.length > 0 && onUpdateTags) {
        activeSelection.forEach((id) => {
          const session = sessionById.get(id);
          const merged = Array.from(new Set([...(session?.tags ?? []), ...newTags]));
          onUpdateTags(id, merged);
        });
      }
      setIsBulkTagEditing(false);
    },
    [onUpdateTags, activeSelection, sessionById]
  );

  // --- Drag lifecycle state -------------------------------------------------------------
  // inFlightDragSessionIds: session IDs currently mid-drag or mid-mutation -- suppresses
  // watchSessions-driven column reassignment for those IDs until the drag/mutation settles
  // or is cancelled (Story 3.2.2). A multi-select drag (Task 6.3.1c) adds every selected
  // session's ID, not just the one the pointer grabbed. dragStartColumnRef snapshots each
  // such session's column at pickup time so the frozen render has something to show.
  // optimisticColumnOverrides holds the *destination* column once a legal move starts
  // mutating, for the "moves immediately, pending visual state until the RPC resolves"
  // behavior (Story 3.1.1).
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

  // Resolves each visible session's current column exactly once, so a session rendered in
  // multiple swimlane rows simultaneously (Tag grouping's multi-membership) always shows --
  // and drags from -- the same column in every row it appears in.
  const columnForSession = useMemo(() => {
    const map = new Map<string, BoardColumnKey>();
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
      map.set(session.id, col);
    }
    return map;
  }, [filteredSessions, optimisticColumnOverrides, inFlightDragSessionIds]);

  const bucketRow = useCallback(
    (sessionsForRow: Session[]): Record<BoardColumnKey, Session[]> => {
      const map: Record<BoardColumnKey, Session[]> = {
        running: [],
        needs_review: [],
        paused: [],
        complete: [],
      };
      for (const session of sessionsForRow) {
        const col = columnForSession.get(session.id) ?? getBoardColumnKey(session);
        map[col].push(session);
      }
      return map;
    },
    [columnForSession]
  );

  // Flat (non-swimlane) bucketing -- used when groupingStrategy === GroupingStrategy.None.
  const buckets = useMemo(() => bucketRow(filteredSessions), [bucketRow, filteredSessions]);

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

  // Task 6.3.1c/d: combined outcome announcement for a multi-select drag fan-out -- reports
  // which sessions (if any) failed to move, rather than only the single dragged session's
  // outcome, per AC8's "surface which failed" partial-bulk-failure requirement.
  const announceMultiOutcome = useCallback(
    (results: { id: string; session: Session; outcome: DragOutcome }[], toColumn: BoardColumnKey) => {
      if (results.length === 0) return;
      const succeeded = results.filter((r) => r.outcome.type === "moved");
      const failed = results.filter((r) => r.outcome.type !== "moved");
      const toLabel = columnLabel(toColumn);

      if (failed.length === 0) {
        announceLive(`${succeeded.length} session${succeeded.length !== 1 ? "s" : ""} moved to ${toLabel}.`);
        return;
      }

      const failedTitles = failed.map((f) => f.session.title).join(", ");
      if (succeeded.length === 0) {
        const message = `Couldn't move ${failed.length} selected session${failed.length !== 1 ? "s" : ""} to ${toLabel}: ${failedTitles}.`;
        showToast(message, "error");
        announceLive(message);
        return;
      }

      const message = `Moved ${succeeded.length} of ${results.length} selected sessions to ${toLabel} — couldn't move: ${failedTitles}.`;
      showToast(message, "warning");
      announceLive(message);
    },
    [columnLabel, showToast, announceLive]
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

      // Multi-select fan-out (Task 6.3.1c): if the picked-up card is part of the current
      // selection AND that selection has more than one member, the whole selection moves --
      // freeze watchSessions reassignment for every selected ID up front, not just the one
      // the pointer grabbed.
      const isMultiSelectDrag = activeSelection.has(sessionId) && activeSelection.size > 1;
      if (isMultiSelectDrag) {
        activeSelection.forEach((id) => {
          const s = sessionById.get(id);
          if (s) beginInFlight(id, getBoardColumnKey(s));
        });
        announceLive(`${activeSelection.size} sessions picked up.`);
      } else {
        beginInFlight(sessionId, getBoardColumnKey(session));
        announceLive(`${session.title} picked up.`);
      }
    },
    [sessionById, activeSelection, announceLive, beginInFlight]
  );

  const handleDragCancel = useCallback(
    (event: DragCancelEvent) => {
      const { entityId: sessionId } = parseCompositeId(String(event.active.id));
      const isMultiSelectDrag = activeSelection.has(sessionId) && activeSelection.size > 1;
      const idsToRelease = isMultiSelectDrag ? Array.from(activeSelection) : [sessionId];
      idsToRelease.forEach((id) => releaseInFlight(id));
      announceOutcome({ type: "cancelled" });
    },
    [activeSelection, releaseInFlight, announceOutcome]
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

      const isMultiSelectDrag = activeSelection.has(sessionId) && activeSelection.size > 1;
      const idsToMove = isMultiSelectDrag ? Array.from(activeSelection) : [sessionId];

      if (!event.over) {
        idsToMove.forEach((id) => releaseInFlight(id));
        announceOutcome({ type: "cancelled" });
        return;
      }

      const { entityId: toColumnId } = parseCompositeId(String(event.over.id));
      const toColumn = toColumnId as BoardColumnKey;

      if (!isMultiSelectDrag) {
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
        return;
      }

      // Multi-select fan-out (Task 6.3.1c): one attemptColumnMove call per selected session,
      // targeting the same toColumn. Sequential (not Promise.all) is deliberate -- a drop into
      // "Complete" opens a single-slot confirmation dialog (confirmComplete/pendingComplete)
      // per session, and awaiting each call in turn keeps those prompts from racing each other.
      const results: { id: string; session: Session; outcome: DragOutcome }[] = [];
      for (const id of idsToMove) {
        const s = sessionById.get(id);
        const from = dragStartColumnRef.current.get(id);
        if (!s || !from) {
          releaseInFlight(id);
          continue;
        }
        if (from === toColumn) {
          releaseInFlight(id);
          continue;
        }

        const outcome = await attemptColumnMove(s, from, toColumn, {
          updateSession,
          resumeHibernatedSession,
          approveNeedsReview,
          confirmComplete,
          getSessionsErrorState,
          onOptimisticMove: (target) => {
            setOptimisticColumnOverrides((prev) => new Map(prev).set(id, target));
          },
        });

        releaseInFlight(id);
        if (outcome.type !== "moved") {
          clearOptimisticOverride(id);
        }
        results.push({ id, session: s, outcome });
      }

      const draggedResult = results.find((r) => r.id === sessionId);
      if (draggedResult?.outcome.type === "moved") {
        pendingFocusRef.current = { sessionId, via: "drag" };
      }
      announceMultiOutcome(results, toColumn);
    },
    [
      sessionById,
      activeSelection,
      releaseInFlight,
      clearOptimisticOverride,
      announceOutcome,
      announceMultiOutcome,
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
  // differs (a discrete menu selection instead of a dnd-kit drop event). MoveToMenu always
  // targets a single session -- it has no concept of "act on the current selection".
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
      selectMode,
      isSelected: activeSelection.has(session.id),
      onToggleSelect: () => handleToggleSession(session.id),
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
      selectMode,
      activeSelection,
      handleToggleSession,
    ]
  );

  return (
    <div className={container} data-context="session-board">
      <div className={boardHeader}>
        <input
          type="text"
          placeholder="Search sessions..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className={`${searchInput} ${boardHeaderSearch}`}
          aria-label="Search sessions"
          data-testid="board-search-input"
        />
        <select
          value={groupingStrategy}
          onChange={(e) => setGroupingStrategy(e.target.value as GroupingStrategy)}
          className={select}
          title="Group by"
          aria-label="Group sessions by"
          data-testid="board-grouping-select"
        >
          {Object.entries(GroupingStrategyLabels).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </select>
        <button
          type="button"
          onClick={handleToggleSelectMode}
          className={`${selectModeButton} ${selectMode ? selectModeButtonActive : ""}`}
          aria-label={selectMode ? "Exit select mode" : "Enter select mode"}
          aria-pressed={selectMode}
          data-testid="board-select-mode-toggle"
        >
          {selectMode ? "Cancel" : "Select"}
        </button>
      </div>

      <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd} onDragCancel={handleDragCancel}>
        <div className={groupingActive ? boardRows : board} role="region" aria-label="Session board">
          {groupingActive
            ? groupedSessions.map((group) => (
                <BoardSwimlane
                  key={group.groupKey}
                  rowKey={group.groupKey}
                  displayName={group.displayName}
                  buckets={bucketRow(group.sessions)}
                  getCardProps={getCardProps}
                />
              ))
            : BOARD_COLUMNS.map((col) => (
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

      {selectMode && (
        <BulkActions
          selectedCount={activeSelection.size}
          totalCount={filteredSessions.length}
          onPauseAll={handlePauseAll}
          onResumeAll={handleResumeAll}
          onDeleteAll={handleDeleteAll}
          onAddTagAll={(e) => handleBulkAddTag(e.currentTarget)}
          onSelectAll={handleSelectAll}
          onClearSelection={handleClearSelection}
        />
      )}

      {isBulkTagEditing && (
        <TagEditor
          tags={[]}
          onSave={handleBulkTagSave}
          onCancel={() => setIsBulkTagEditing(false)}
          sessionTitle={`${activeSelection.size} selected session${activeSelection.size !== 1 ? "s" : ""}`}
          triggerRef={bulkTagEditorTriggerRef}
        />
      )}
    </div>
  );
}
