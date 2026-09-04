"use client";

import { useRef, useEffect } from "react";
import type { Session } from "@/gen/session/v1/types_pb";
import { SessionDetail } from "@/components/sessions/SessionDetail";
import { SessionList } from "@/components/sessions/SessionList";
import { SessionBoard } from "@/components/sessions/SessionBoard";
import { SessionListSkeleton } from "@/components/sessions/SessionListSkeleton";
import {
  skeletonRow,
  dot,
  nameBar,
  agentPlaceholder,
  pathBar,
  timeBar,
  actionsSpacer,
} from "@/components/sessions/SessionListSkeleton.css";
import { ErrorState } from "@/components/ui/ErrorState";
import { useSessionViewModeContext } from "@/lib/contexts/SessionViewModeContext";
import { BOARD_COLUMNS } from "@/lib/board/columns";
import type { PaneNode, LeafPane, SplitPane, PaneState, PaneAction, PaneId, SessionDetailTab, PaneViewKind } from "@/lib/pane/paneTypes";
import { getAllLeaves } from "@/lib/pane/paneReducer";
import { useCockpitActions } from "@/lib/contexts/CockpitActionsContext";
import { useSessionServiceContext } from "@/lib/contexts/SessionServiceContext";
import { usePaneContext } from "./PaneContext";
import { PaneHeader } from "./PaneHeader";
import { ResizeHandle } from "./ResizeHandle";
import { MobilePaneTabStrip } from "./MobilePaneTabStrip";
import { useViewport } from "@/components/providers/ViewportProvider";
import { containsPaneId, hasVerticalSplit } from "@/lib/pane/paneUtils";
import {
  splitContainer,
  leafContainer,
  leafZoomed,
  emptyPaneSlot,
  paneBody,
  sessionListScroll,
  resetLayoutBar,
  resetLayoutButton,
  viewModeToggleBar,
  viewModeToggleButton,
  rendererRoot,
  rendererContent,
} from "@/styles/pane/paneSplit.css";
import { pickerOverlay, pickerLabel } from "@/styles/pane/panePickerOverlay.css";

function getPickerLetter(root: PaneNode, paneId: string): string | null {
  const allLeaves = getAllLeaves(root);
  const eligible = allLeaves.filter((l) => l.viewKind !== "session-list");
  const idx = eligible.findIndex((l) => l.id === paneId);
  return idx >= 0 && idx < 26 ? "ABCDEFGHIJKLMNOPQRSTUVWXYZ"[idx] : null;
}

const BOARD_SKELETON_ROWS_PER_COLUMN = 3;

// Board-shaped loading placeholder — 4 column shells with shimmer rows, shown instead of
// the flat SessionListSkeleton while in board view, so first-load never looks identical to
// a genuinely empty board (ux.md "Loading state", AC31). Reuses SessionListSkeleton's
// shimmer row pieces rather than BoardColumn's own markup/CSS, since BoardColumn.tsx is
// owned by the concurrent Phase 3 work.
function BoardColumnsSkeleton() {
  return (
    <div
      role="status"
      aria-busy="true"
      aria-label="Loading sessions…"
      data-testid="board-columns-skeleton"
      style={{ display: "flex", gap: 16, padding: 16, flex: 1, overflowX: "auto", minHeight: 0 }}
    >
      {BOARD_COLUMNS.map((col) => (
        <div
          key={col.key}
          data-testid={`board-column-skeleton-${col.key}`}
          style={{ width: 320, flexShrink: 0 }}
        >
          <div style={{ fontSize: 12, fontWeight: 600, opacity: 0.6, marginBottom: 8, textTransform: "uppercase" }}>
            {col.label}
          </div>
          {Array.from({ length: BOARD_SKELETON_ROWS_PER_COLUMN }, (_, i) => (
            <div key={i} className={skeletonRow} aria-hidden="true" style={{ marginBottom: 8 }}>
              <div className={dot} />
              <div className={nameBar} />
              <div className={agentPlaceholder} />
              <div className={pathBar} />
              <div className={timeBar} />
              <div className={actionsSpacer} />
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}

interface PaneSplitRendererProps {
  state: PaneState;
  dispatch: React.Dispatch<PaneAction>;
  sessions: Session[];
}

interface PaneNodeProps {
  node: PaneNode;
  state: PaneState;
  dispatch: React.Dispatch<PaneAction>;
  sessions: Session[];
  isMobile: boolean;
  hasSplits: boolean;
}

function PaneNodeComponent({ node, state, dispatch, sessions, isMobile, hasSplits }: PaneNodeProps) {
  if (node.type === "leaf") {
    return (
      <PaneLeafComponent
        pane={node}
        state={state}
        dispatch={dispatch}
        sessions={sessions}
        isMobile={isMobile}
        hasSplits={hasSplits}
      />
    );
  }

  return (
    <PaneSplitComponent
      pane={node}
      state={state}
      dispatch={dispatch}
      sessions={sessions}
      isMobile={isMobile}
      hasSplits={hasSplits}
    />
  );
}

interface PaneSplitProps {
  pane: SplitPane;
  state: PaneState;
  dispatch: React.Dispatch<PaneAction>;
  sessions: Session[];
  isMobile: boolean;
  hasSplits: boolean;
}

function PaneSplitComponent({ pane, state, dispatch, sessions, isMobile, hasSplits }: PaneSplitProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  // On mobile: vertical (side-by-side) splits collapse to the focused pane only.
  // Horizontal (top/bottom) splits are fine on mobile — show both panes stacked.
  if (isMobile && pane.direction === "vertical") {
    const focusedInFirst = containsPaneId(pane.first, state.focusedPaneId);
    const visibleNode = focusedInFirst ? pane.first : pane.second;
    return (
      <PaneNodeComponent
        node={visibleNode}
        state={state}
        dispatch={dispatch}
        sessions={sessions}
        isMobile={isMobile}
        hasSplits={hasSplits}
      />
    );
  }

  return (
    <div
      ref={containerRef}
      className={splitContainer({ direction: pane.direction })}
      style={{ "--split-ratio": String(pane.ratio) } as React.CSSProperties}
    >
      <PaneNodeComponent
        node={pane.first}
        state={state}
        dispatch={dispatch}
        sessions={sessions}
        isMobile={isMobile}
        hasSplits={hasSplits}
      />
      {/* Draggable resize is a desktop affordance — touch dragging a 6px divider is
          impractical, and mobile horizontal splits use a fixed 50/50 ratio instead. */}
      {!isMobile && (
        <ResizeHandle
          splitId={pane.id}
          direction={pane.direction}
          onResize={(splitId: PaneId, ratio: number) =>
            dispatch({ type: "RESIZE_PANE", splitId, ratio })
          }
        />
      )}
      <PaneNodeComponent
        node={pane.second}
        state={state}
        dispatch={dispatch}
        sessions={sessions}
        isMobile={isMobile}
        hasSplits={hasSplits}
      />
    </div>
  );
}

interface PaneLeafProps {
  pane: LeafPane;
  state: PaneState;
  dispatch: React.Dispatch<PaneAction>;
  sessions: Session[];
  isMobile: boolean;
  hasSplits: boolean;
}

function SessionListPaneBody({ pane, dispatch }: { pane: LeafPane; dispatch: React.Dispatch<PaneAction> }) {
  const actions = useCockpitActions();
  const { sessions, loading, error, listSessions, hibernateSession, resumeHibernatedSession } = useSessionServiceContext();
  const { triggerPicker, triggerPickerForceNew } = usePaneContext();
  const { viewMode, setViewMode } = useSessionViewModeContext();
  if (loading) return viewMode === "board" ? <BoardColumnsSkeleton /> : <SessionListSkeleton count={4} />;
  // `error` is useSessionServiceContext's single shared Redux field -- ANY
  // session-mutating RPC (createSession, updateSession, cancelSessionCreation,
  // etc., see useSessionService.ts's dispatch(setError(...)) call sites)
  // writes to this same slot, not just the initial list fetch. Gating the
  // full-pane "Failed to Load Sessions" fallback on `error` alone means an
  // already-successfully-loaded, non-empty list gets replaced by that
  // fallback the moment ANY later mutation fails -- e.g. a routine,
  // client-visible rejection like Omnibar's duplicate-title create error
  // (session-creation-async.spec.ts's "duplicate title keeps the omnibar
  // open with inline error" regression test), which already has its own
  // inline `omnibar-create-error` alert and needs no page-level fallback at
  // all. Only show this fallback when there is nothing else to show --
  // i.e. the list itself is empty because the fetch that would have
  // populated it is what failed.
  if (error && sessions.length === 0) {
    return (
      <ErrorState
        error={error}
        title="Failed to Load Sessions"
        message="Unable to connect to the server."
        onRetry={listSessions}
      />
    );
  }

  const sharedProps = {
    sessions,
    onSessionClick: triggerPicker,
    onSessionOpenInNewPane: triggerPickerForceNew,
    onDeleteSession: actions.onDeleteSession,
    onPauseSession: actions.onPauseSession,
    onResumeSession: actions.onResumeSession,
    onDirectResumeSession: actions.onDirectResumeSession,
    onCloneSession: actions.onCloneSession,
    onNewWorkspaceSession: actions.onNewWorkspaceSession,
    onRenameSession: actions.onRenameSession,
    onRestartSession: actions.onRestartSession,
    onUpdateTags: actions.onUpdateTags,
    onNewSession: actions.onNewSession,
    onCreateCheckpoint: actions.onCreateCheckpoint,
    onListCheckpoints: actions.onListCheckpoints,
    onForkFromCheckpoint: actions.onForkFromCheckpoint,
    onSetRateLimitEnabled: actions.onSetRateLimitEnabled,
    onToggleAutonomousMode: actions.onToggleAutonomousMode,
    onToggleAutoApprove: actions.onToggleAutoApprove,
    onSteerAutonomousSession: actions.onSteerAutonomousSession,
    onClearConversationState: actions.onClearConversationState,
    onHibernateSession: hibernateSession ? (id: string) => void hibernateSession(id) : undefined,
    onResumeHibernatedSession: resumeHibernatedSession ? (id: string) => void resumeHibernatedSession(id) : undefined,
    onFetchArchivedSessions: (includeArchived: boolean) => /* analytics-exempt */ void listSessions({ includeArchived }),
    storageKeyPrefix: `pane-${pane.id}.`,
  };

  return (
    <>
      <div className={viewModeToggleBar} role="group" aria-label="Session view">
        <button
          type="button"
          data-testid="session-view-mode-list"
          className={viewModeToggleButton({ active: viewMode === "list" })}
          aria-pressed={viewMode === "list"}
          onClick={() => setViewMode("list")}
        >
          List
        </button>
        <button
          type="button"
          data-testid="session-view-mode-board"
          className={viewModeToggleButton({ active: viewMode === "board" })}
          aria-pressed={viewMode === "board"}
          onClick={() => setViewMode("board")}
        >
          Board
        </button>
      </div>
      {/* Announces the view switch to screen-reader users, who otherwise get no
          feedback that the toggle buttons above changed anything. */}
      <div
        role="status"
        aria-live="polite"
        aria-atomic="true"
        style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0,0,0,0)" }}
      >
        {viewMode === "board" ? `Board view, showing ${sessions.length} sessions` : `List view, showing ${sessions.length} sessions`}
      </div>
      <div className={sessionListScroll} data-testid="session-list-scroll">
        {viewMode === "board" ? (
          <SessionBoard {...sharedProps} />
        ) : (
          <SessionList {...sharedProps} />
        )}
      </div>
    </>
  );
}

function PaneLeafComponent({ pane, state, dispatch, sessions, isMobile, hasSplits }: PaneLeafProps) {
  const { pickerPendingSession, cancelPicker } = usePaneContext();
  const isFocused = state.focusedPaneId === pane.id;
  const isZoomed = state.zoomedPaneId === pane.id;
  const pickerLetter = pickerPendingSession ? getPickerLetter(state.root, pane.id) : null;
  const session = pane.viewKind === "session-detail" && pane.sessionId
    ? sessions.find((s) => s.id === pane.sessionId) ?? null
    : null;

  const handleFocus = () => dispatch({ type: "FOCUS_PANE", paneId: pane.id });
  const handleClose = () => dispatch({ type: "CLOSE_PANE", paneId: pane.id });
  const handleZoom = () => dispatch({ type: "ZOOM_PANE", paneId: pane.id });
  const handleTabChange = (tab: SessionDetailTab) =>
    dispatch({ type: "ASSIGN_TAB", paneId: pane.id, tab });
  const handleSplitVertical = () =>
    dispatch({ type: "SPLIT_PANE", paneId: pane.id, direction: "vertical" });
  const handleSplitHorizontal = () =>
    dispatch({ type: "SPLIT_PANE", paneId: pane.id, direction: "horizontal" });
  const handleSetView = (viewKind: PaneViewKind) =>
    dispatch({ type: "SET_PANE_VIEW", paneId: pane.id, viewKind });

  const handleDragStart = (e: React.DragEvent) => {
    e.dataTransfer.setData("text/pane-id", pane.id);
    e.dataTransfer.effectAllowed = "move";
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    const sourcePaneId = e.dataTransfer.getData("text/pane-id");
    if (sourcePaneId && sourcePaneId !== pane.id) {
      dispatch({ type: "SWAP_PANES", paneId: sourcePaneId, targetPaneId: pane.id });
    }
  };

  return (
    <div
      className={`${leafContainer({ focused: isFocused && hasSplits })}${isZoomed ? ` ${leafZoomed}` : ""}`}
      data-focused={isFocused ? "true" : "false"}
      data-testid={`pane-leaf-${pane.id}`}
      data-context="cockpit"
      draggable={hasSplits}
      onDragStart={handleDragStart}
      onDragOver={handleDragOver}
      onDrop={handleDrop}
      onClick={handleFocus}
    >
      <PaneHeader
        pane={pane}
        sessions={sessions}
        isFocused={isFocused}
        onClose={handleClose}
        onFocus={handleFocus}
        onZoom={handleZoom}
        onSetView={handleSetView}
        splitButtonVisible={!isMobile}
        onSplitVertical={handleSplitVertical}
        onSplitHorizontal={handleSplitHorizontal}
      />
      <div className={paneBody}>
        {pane.viewKind === "session-list" ? (
          <SessionListPaneBody pane={pane} dispatch={dispatch} />
        ) : session ? (
          <SessionDetail
            key={`${pane.id}-${pane.sessionId}`}
            session={session}
            onClose={handleClose}
            onFullscreenChange={() => {}}
            onTabChange={handleTabChange}
            initialTab={pane.activeTab}
            embedded={true}
          />
        ) : (
          <div className={emptyPaneSlot}>
            Click a session to open it here
          </div>
        )}
      </div>
      {pickerLetter && (
        <div
          className={pickerOverlay}
          onClick={(e) => {
            e.stopPropagation();
            dispatch({ type: "ASSIGN_SESSION", paneId: pane.id, sessionId: pickerPendingSession!.id });
            dispatch({ type: "ASSIGN_TAB", paneId: pane.id, tab: "terminal" });
            cancelPicker();
          }}
          aria-label={`Open session in this pane (press ${pickerLetter})`}
          role="button"
          tabIndex={0}
        >
          <span className={pickerLabel}>{pickerLetter}</span>
        </div>
      )}
    </div>
  );
}

/**
 * PaneSplitRenderer — root renderer for the pane tree.
 * Renders the full recursive pane layout.
 */
export function PaneSplitRenderer({ state, dispatch, sessions }: PaneSplitRendererProps) {
  const { isMobile, isFoldable } = useViewport();
  // Collapse split panes for any viewport below 900px (mobile + foldable) — BottomNav is
  // visible there and single-pane + MobilePaneTabStrip gives a better UX than cramped splits.
  const isNarrow = isMobile || isFoldable;
  const allLeaves = getAllLeaves(state.root);
  const hasMultiplePanes = allLeaves.length > 1;
  // Show tab strip on narrow screens whenever there are multiple panes — vertical splits
  // collapse to one visible pane and need the strip to switch; horizontal splits keep both
  // visible but still benefit from the strip's "+" add-pane button.
  const showMobileTabStrip = isNarrow && hasMultiplePanes;

  // Publish the actual strip height via ResizeObserver so fixed-position elements
  // (e.g. notification toasts) can clear it without hard-coding the pixel value.
  const stripRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!showMobileTabStrip) {
      document.documentElement.style.setProperty("--mobile-pane-tab-strip-height", "0px");
      return;
    }
    const el = stripRef.current;
    if (!el) return;
    const update = () => {
      document.documentElement.style.setProperty(
        "--mobile-pane-tab-strip-height",
        `${el.offsetHeight}px`
      );
    };
    const ro = new ResizeObserver(update);
    ro.observe(el);
    update();
    return () => ro.disconnect();
  }, [showMobileTabStrip]);

  return (
    <div
      className={rendererRoot}
      data-context="cockpit"
    >
      {/* Reset layout button — desktop window-management chrome; on mobile the
          MobilePaneTabStrip + per-pane close buttons cover the same need without
          eating scarce header space. */}
      {hasMultiplePanes && !isNarrow && (
        <div className={resetLayoutBar}>
          <button
            data-testid="reset-layout-btn"
            className={resetLayoutButton}
            onClick={() => dispatch({ type: "RESET_LAYOUT" })}
            title="Reset to single pane"
          >
            Reset layout
          </button>
        </div>
      )}

      <div className={rendererContent}>
        <PaneNodeComponent
          node={state.root}
          state={state}
          dispatch={dispatch}
          sessions={sessions}
          isMobile={isNarrow}
          hasSplits={hasMultiplePanes}
        />
      </div>

      {showMobileTabStrip && (
        <MobilePaneTabStrip
          ref={stripRef}
          leaves={allLeaves}
          focusedPaneId={state.focusedPaneId}
          sessions={sessions}
          onFocus={(paneId: PaneId) => dispatch({ type: "FOCUS_PANE", paneId })}
          onAddPane={() => dispatch({ type: "SPLIT_PANE", paneId: state.focusedPaneId, direction: "horizontal" })}
        />
      )}
    </div>
  );
}
