"use client";

import { LayoutList, LayoutDashboard, Columns2, Rows2, Maximize2, X } from "lucide-react";
import type { Session } from "@/gen/session/v1/types_pb";
import type { LeafPane, SessionDetailTab, PaneViewKind } from "@/lib/pane/paneTypes";
import { useCockpitActions } from "@/lib/contexts/CockpitActionsContext";
import { SessionActionsOverflow } from "@/components/sessions/SessionActionsOverflow";
import {
  paneHeader,
  paneTitle,
  paneHeaderButton,
  paneCloseButton,
  paneTabButton,
} from "@/styles/pane/paneHeader.css";

const TAB_LABELS: Record<SessionDetailTab, string> = {
  terminal: "Term",
  diff: "Diff",
  vcs: "VCS",
  logs: "Logs",
  info: "Info",
  files: "Files",
};

const TAB_FULL_LABELS: Record<SessionDetailTab, string> = {
  terminal: "Terminal",
  diff: "Diff",
  vcs: "Version Control",
  logs: "Logs",
  info: "Session Info",
  files: "Files",
};

const ALL_TABS: SessionDetailTab[] = ["terminal", "diff", "vcs", "logs", "info", "files"];

interface PaneHeaderProps {
  pane: LeafPane;
  sessions: Session[];
  isFocused: boolean;
  onClose: () => void;
  onFocus: () => void;
  onTabChange: (tab: SessionDetailTab) => void;
  onZoom: () => void;
  onSetView?: (viewKind: PaneViewKind) => void;
  splitButtonVisible?: boolean;
  onSplitVertical?: () => void;
  onSplitHorizontal?: () => void;
}

export function PaneHeader({
  pane,
  sessions,
  isFocused: _isFocused,
  onClose,
  onFocus,
  onTabChange,
  onZoom,
  onSetView,
  splitButtonVisible,
  onSplitVertical,
  onSplitHorizontal,
}: PaneHeaderProps) {
  const cockpit = useCockpitActions();
  const isListPane = pane.viewKind === "session-list";
  const session = !isListPane && pane.sessionId
    ? sessions.find((s) => s.id === pane.sessionId) ?? null
    : null;
  const titleText = isListPane ? "Sessions" : (session ? session.title : "Empty");

  return (
    <div
      className={paneHeader}
      data-testid={`pane-header-${pane.id}`}
      onClick={onFocus}
    >
      <span className={paneTitle} title={titleText}>
        {titleText}
      </span>

      {/* Tab switcher buttons (only for session-detail panes with a session) */}
      {!isListPane && session &&
        ALL_TABS.map((tab) => (
          <button
            key={tab}
            className={paneTabButton({ active: pane.activeTab === tab })}
            onClick={(e) => {
              e.stopPropagation();
              onTabChange(tab);
            }}
            title={TAB_FULL_LABELS[tab]}
            aria-label={`Switch to ${TAB_FULL_LABELS[tab]} tab`}
          >
            {TAB_LABELS[tab]}
          </button>
        ))}

      {/* Session actions overflow menu */}
      {!isListPane && session && (
        <div onClick={(e) => e.stopPropagation()}>
          <SessionActionsOverflow
            session={session}
            buttonClassName={paneHeaderButton}
            onPause={() => cockpit.onPauseSession(session.id)}
            onResume={() => cockpit.onResumeSession(session)}
            onDelete={() => cockpit.onDeleteSession(session.id)}
            onRestart={(id) => cockpit.onRestartSession(id)}
            onClone={() => cockpit.onCloneSession(session.id)}
            onNewWorkspace={() => cockpit.onNewWorkspaceSession(session.id)}
            onCreateCheckpoint={(id, label) => cockpit.onCreateCheckpoint(id, label)}
            onRunOneShot={(id) => cockpit.onRunOneShot(id)}
            onSetRateLimitEnabled={(id, enabled) => cockpit.onSetRateLimitEnabled(id, enabled)}
            onClearConversationState={(id) => cockpit.onClearConversationState(id)}
            onUpdateTags={(id, tags) => cockpit.onUpdateTags(id, tags)}
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
        >
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
          >
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
          >
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
        >
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
      >
        <X size={14} />
      </button>
    </div>
  );
}
