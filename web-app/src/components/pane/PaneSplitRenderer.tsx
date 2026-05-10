"use client";

import { useRef } from "react";
import type { Session } from "@/gen/session/v1/types_pb";
import { SessionDetail } from "@/components/sessions/SessionDetail";
import { SessionList } from "@/components/sessions/SessionList";
import { SessionListSkeleton } from "@/components/sessions/SessionListSkeleton";
import { ErrorState } from "@/components/ui/ErrorState";
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
      <ResizeHandle
        splitId={pane.id}
        direction={pane.direction}
        onResize={(splitId: PaneId, ratio: number) =>
          dispatch({ type: "RESIZE_PANE", splitId, ratio })
        }
      />
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
  hasSplits: boolean;
}

function SessionListPaneBody({ pane, dispatch }: { pane: LeafPane; dispatch: React.Dispatch<PaneAction> }) {
  const actions = useCockpitActions();
  const { sessions, loading, error, listSessions } = useSessionServiceContext();
  const { triggerPicker, triggerPickerForceNew } = usePaneContext();
  if (loading) return <SessionListSkeleton count={4} />;
  if (error) {
    return (
      <ErrorState
        error={error}
        title="Failed to Load Sessions"
        message="Unable to connect to the server."
        onRetry={listSessions}
      />
    );
  }
  return (
    <div className={sessionListScroll} data-testid="session-list-scroll">
      <SessionList
        sessions={sessions}
        onSessionClick={triggerPicker}
        onSessionOpenInNewPane={triggerPickerForceNew}
        onDeleteSession={actions.onDeleteSession}
        onPauseSession={actions.onPauseSession}
        onResumeSession={actions.onResumeSession}
        onDirectResumeSession={actions.onDirectResumeSession}
        onCloneSession={actions.onCloneSession}
        onNewWorkspaceSession={actions.onNewWorkspaceSession}
        onRenameSession={actions.onRenameSession}
        onRestartSession={actions.onRestartSession}
        onUpdateTags={actions.onUpdateTags}
        onNewSession={actions.onNewSession}
        onCreateCheckpoint={actions.onCreateCheckpoint}
        onListCheckpoints={actions.onListCheckpoints}
        onForkFromCheckpoint={actions.onForkFromCheckpoint}
        onRunOneShot={actions.onRunOneShot}
        onSetRateLimitEnabled={actions.onSetRateLimitEnabled}
        onClearConversationState={actions.onClearConversationState}
        storageKeyPrefix={`pane-${pane.id}.`}
      />
    </div>
  );
}

function PaneLeafComponent({ pane, state, dispatch, sessions, hasSplits }: PaneLeafProps) {
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
        onTabChange={handleTabChange}
        onZoom={handleZoom}
        onSetView={handleSetView}
        splitButtonVisible={true}
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

  return (
    <div
      className={rendererRoot}
      data-context="cockpit"
    >
      {/* Reset layout button — only shown when there is a split layout */}
      {hasMultiplePanes && (
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
