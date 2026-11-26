"use client";

import { LayoutList, LayoutDashboard, Columns2, Rows2, Maximize2, X, GripVertical } from "lucide-react";
import type { Session } from "@/gen/session/v1/types_pb";
import type { LeafPane, PaneViewKind } from "@/lib/pane/paneTypes";
import { useCockpitActions } from "@/lib/contexts/CockpitActionsContext";
import { SessionActionsOverflow } from "@/components/sessions/SessionActionsOverflow";
import { BacklogOriginBadge } from "@/components/shared/BacklogOriginBadge";
import { useSessionActions } from "@/lib/hooks/useSessionActions";
import { usePaneContext } from "./PaneContext";
import { titleFirstLine } from "@/lib/pane/paneUtils";
import {
  paneHeader,
  paneTitle,
  paneHeaderButton,
  paneHeaderActions,
  paneCloseButton,
  paneDragHandle,
} from "@/styles/pane/paneHeader.css";

interface PaneHeaderProps {
  pane: LeafPane;
  sessions: Session[];
  isFocused: boolean;
  onClose: () => void;
  onFocus: () => void;
  onZoom: () => void;
  onSetView?: (viewKind: PaneViewKind) => void;
  splitButtonVisible?: boolean;
  onSplitVertical?: () => void;
  onSplitHorizontal?: () => void;
  /** When set, a grip is shown and dragging it starts a pane swap. */
  onDragStart?: (e: React.DragEvent) => void;
}

export function PaneHeader({
  pane,
  sessions,
  isFocused: _isFocused,
  onClose,
  onFocus,
  onZoom,
  onSetView,
  splitButtonVisible,
  onSplitVertical,
  onSplitHorizontal,
  onDragStart,
}: PaneHeaderProps) {
  const cockpit = useCockpitActions();
  const { backlogIndex, triggerPickerForceNew } = usePaneContext();
  const isListPane = pane.viewKind === "session-list";
  const session = !isListPane && pane.sessionId
    ? sessions.find((s) => s.id === pane.sessionId) ?? null
    : null;
  const sessionActions = useSessionActions(session?.id ?? "");
  const titleText = isListPane ? "Sessions" : (session ? titleFirstLine(session.title) : "Empty");

  return (
    <div
      className={paneHeader}
      data-testid={`pane-header-${pane.id}`}
      onClick={onFocus}
    >
      {onDragStart && (
        <span
          className={paneDragHandle}
          draggable
          onDragStart={onDragStart}
          onClick={(e) => e.stopPropagation()}
          title="Drag to swap with another pane"
          role="img"
          aria-label="Drag to swap panes"
          data-testid={`pane-drag-handle-${pane.id}`}
        >
          <GripVertical size={14} aria-hidden="true" />
        </span>
      )}
      <span className={paneTitle} title={session?.title ?? titleText}>
        {titleText}
      </span>
      {session && <BacklogOriginBadge entry={backlogIndex.get(session.id)} compact />}

      <div className={paneHeaderActions}>
        {/* Session actions overflow menu */}
        {!isListPane && session && (
          <div onClick={(e) => e.stopPropagation()}>
            <SessionActionsOverflow
              session={session}
              buttonClassName={paneHeaderButton}
              onPause={() => cockpit.onPauseSession(session.id)}
              onResume={() => cockpit.onResumeSession(session)}
              onHibernate={() => cockpit.onHibernateSession(session.id)}
              onResumeFromHibernation={() => cockpit.onResumeFromHibernation(session.id)}
              onDelete={() => cockpit.onDeleteSession(session.id)}
              onRestart={(id) => cockpit.onRestartSession(id)}
              onRetryNow={(id) => cockpit.onRetryNowSession(id)}
              onArchive={(id) => cockpit.onArchiveSession(id)}
              onUnarchive={(id) => cockpit.onUnarchiveSession(id)}
              onClone={() => cockpit.onCloneSession(session.id)}
              onOpenInNewPane={() => triggerPickerForceNew(session)}
              onNewWorkspace={() => cockpit.onNewWorkspaceSession(session.id)}
              onCreateCheckpoint={(id, label) => cockpit.onCreateCheckpoint(id, label)}
              onSetRateLimitEnabled={(id, enabled) => cockpit.onSetRateLimitEnabled(id, enabled)}
              onToggleAutonomousMode={(id, enabled) => cockpit.onToggleAutonomousMode(id, enabled)}
              onTogglePinned={(id, pinned) => cockpit.onTogglePinned(id, pinned)}
              onToggleAutoApprove={(id, enabled) => cockpit.onToggleAutoApprove(id, enabled)}
              onSteerAutonomousSession={(id, msg) => cockpit.onSteerAutonomousSession(id, msg)}
              onClearConversationState={(id) => cockpit.onClearConversationState(id)}
              onUpdateTags={(id, tags) => cockpit.onUpdateTags(id, tags)}
              onChangeProgram={async (_id, program) => {
                const result = await sessionActions.update({ program });
                if (!result) throw new Error("Failed to change program.");
              }}
            />
          </div>
        )}

        {/* View-kind toggle: cycle between session-list and session-detail */}
        {onSetView && (
          <button
            className={paneHeaderButton}
            data-testid="pane-view-toggle-btn"
            onClick={(e) => {
              e.stopPropagation();
              onSetView(isListPane ? "session-detail" : "session-list");
            }}
            title={isListPane ? "Switch to session detail" : "Switch to session list"}
            aria-label={isListPane ? "Switch to session detail" : "Switch to session list"}
          >{/* analytics-exempt */}
            {isListPane ? <LayoutDashboard size={14} /> : <LayoutList size={14} />}
          </button>
        )}

        {/* Split buttons */}
        {splitButtonVisible && (
          <>
            <button
              className={paneHeaderButton}
              data-testid="pane-split-vertical-btn"
              onClick={(e) => {
                e.stopPropagation();
                onSplitVertical?.();
              }}
              title="Split pane side by side (vertical)"
              aria-label="Split pane side by side"
            >{/* analytics-exempt */}
              <Columns2 size={14} />
            </button>
            <button
              className={paneHeaderButton}
              data-testid="pane-split-horizontal-btn"
              onClick={(e) => {
                e.stopPropagation();
                onSplitHorizontal?.();
              }}
              title="Split pane top and bottom (horizontal)"
              aria-label="Split pane top and bottom"
            >{/* analytics-exempt */}
              <Rows2 size={14} />
            </button>
          </>
        )}

        {/* Zoom button (only for session-detail) */}
        {!isListPane && (
          <button
            className={paneHeaderButton}
            onClick={(e) => {
              e.stopPropagation();
              onZoom();
            }}
            title="Fullscreen pane"
            aria-label="Fullscreen pane"
          >{/* analytics-exempt */}
            <Maximize2 size={14} />
          </button>
        )}

        {/* Close button */}
        <button
          className={paneCloseButton}
          onClick={(e) => {
            e.stopPropagation();
            onClose();
          }}
          title="Close pane"
          aria-label="Close pane"
        >{/* analytics-exempt */}
          <X size={14} />
        </button>
      </div>
    </div>
  );
}
