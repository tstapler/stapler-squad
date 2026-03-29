"use client";

import { useState, useEffect } from "react";
import dynamic from "next/dynamic";
import { Session, InstanceType } from "@/gen/session/v1/types_pb";
import { DiffViewer } from "./DiffViewer";
import { VcsPanel } from "./VcsPanel";
import { WorkspaceSwitchModal } from "./WorkspaceSwitchModal";
import { ApprovalPanel } from "./ApprovalPanel";
import { SessionLogsTab } from "./SessionLogsTab";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { getApiBaseUrl } from "@/lib/config";
import { getProgramDisplay, isKnownProgram, PROGRAMS } from "@/lib/constants/programs";
import styles from "./SessionDetail.module.css";

// Dynamically import TerminalOutput with SSR disabled (xterm.js requires browser environment)
const TerminalOutput = dynamic(
  () => import("./TerminalOutput").then((mod) => mod.TerminalOutput),
  {
    ssr: false,
    loading: () => (
      <div style={{ padding: "20px", textAlign: "center" }}>
        Loading terminal...
      </div>
    ),
  }
);

export type SessionDetailTab = "terminal" | "diff" | "vcs" | "logs" | "info";

interface SessionDetailProps {
  session: Session;
  onClose: () => void;
  onFullscreenChange?: (isFullscreen: boolean) => void;
  onTabChange?: (tab: SessionDetailTab) => void;
  initialTab?: SessionDetailTab;
  onNext?: () => void; // Navigate to next item in review queue
  onPrevious?: () => void; // Navigate to previous item in review queue
  showNavigation?: boolean; // Show navigation arrows (for review queue)
  onApprovalResolved?: () => void; // Auto-advance callback when all approvals are resolved
  onDismissFromQueue?: () => void; // Acknowledge current session and advance to next
  queuePosition?: number; // 1-indexed position in the review queue (0 = not in queue)
  queueTotal?: number; // Total items in the review queue
}

export function SessionDetail({
  session,
  onClose,
  onFullscreenChange,
  onTabChange,
  initialTab = "info",
  onNext,
  onPrevious,
  showNavigation = false,
  onApprovalResolved,
  onDismissFromQueue,
  queuePosition,
  queueTotal,
}: SessionDetailProps) {
  const [activeTab, setActiveTab] = useState<SessionDetailTab>(initialTab);
  const [isFullscreen, setIsFullscreen] = useState(initialTab === "terminal" || initialTab === "diff" || initialTab === "vcs");
  const [showWorkspaceSwitchModal, setShowWorkspaceSwitchModal] = useState(false);
  const [isEditingProgram, setIsEditingProgram] = useState(false);
  const [programValue, setProgramValue] = useState(session.program || "");
  const { updateSession } = useSessionService();

  // Notify parent of fullscreen state changes
  useEffect(() => {
    onFullscreenChange?.(isFullscreen);
  }, [isFullscreen, onFullscreenChange]);

  const tabs: { id: SessionDetailTab; label: string; icon: string }[] = [
    { id: "terminal", label: "Terminal", icon: "⌨️" },
    { id: "diff", label: "Diff", icon: "📝" },
    { id: "vcs", label: "VCS", icon: "🌿" },
    { id: "logs", label: "Logs", icon: "📋" },
    { id: "info", label: "Info", icon: "ℹ️" },
  ];

  const handleTabChange = (tabId: SessionDetailTab) => {
    setActiveTab(tabId);
    onTabChange?.(tabId);
    // Automatically enter fullscreen for terminal, diff, and vcs tabs
    if (tabId === "terminal" || tabId === "diff" || tabId === "vcs") {
      setIsFullscreen(true);
    } else {
      setIsFullscreen(false);
    }
  };

  // Keyboard shortcuts: Escape to exit fullscreen, Shift+Arrow for navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isFullscreen) {
        setIsFullscreen(false);
      }

      // Review queue navigation shortcuts (Shift+Arrow)
      if (showNavigation && e.shiftKey && !e.ctrlKey && !e.altKey && !e.metaKey) {
        if (e.key === "ArrowRight" && onNext) {
          e.preventDefault();
          onNext();
        } else if (e.key === "ArrowLeft" && onPrevious) {
          e.preventDefault();
          onPrevious();
        }
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isFullscreen, showNavigation, onNext, onPrevious]);

  // Handler for saving program change
  const handleSaveProgram = async () => {
    if (programValue !== session.program) {
      await updateSession(session.id, { program: programValue });
    }
    setIsEditingProgram(false);
  };

  // Handler for canceling program edit
  const handleCancelProgramEdit = () => {
    setProgramValue(session.program || "");
    setIsEditingProgram(false);
  };

  return (
    <div className={`${styles.container} ${isFullscreen ? styles.fullscreen : ""}`}>
      <div className={styles.header}>
        <h2 className={styles.title}>{session.title}</h2>
        <div className={styles.headerActions}>
          {session.instanceType !== InstanceType.EXTERNAL && (
            <button
              className={styles.switchWorkspaceButton}
              onClick={() => setShowWorkspaceSwitchModal(true)}
              aria-label="Switch workspace"
              title="Switch branch, bookmark, or worktree"
            >
              ⎇ Switch
            </button>
          )}
          {(activeTab === "terminal" || activeTab === "diff" || activeTab === "vcs") && (
            <button
              className={styles.fullscreenButton}
              onClick={() => setIsFullscreen(!isFullscreen)}
              aria-label={isFullscreen ? "Exit fullscreen" : "Enter fullscreen"}
              title={isFullscreen ? "Exit fullscreen (Esc)" : "Enter fullscreen"}
            >
              {isFullscreen ? "⊗" : "⛶"}
            </button>
          )}
          {onDismissFromQueue && (
            <button
              className={styles.navButton}
              onClick={onDismissFromQueue}
              aria-label="Clear from queue and advance"
              title="Clear from queue and advance to next"
            >
              ⏭
            </button>
          )}
          {showNavigation && (
            <>
              {queuePosition !== undefined && queuePosition > 0 && queueTotal !== undefined && queueTotal > 0 && (
                <span className={styles.queuePosition} aria-live="polite">
                  {queuePosition} of {queueTotal}
                </span>
              )}
              <button
                className={styles.navButton}
                onClick={onPrevious}
                aria-label="Previous session"
                title="Previous session (Shift+←)"
              >
                ←
              </button>
              <button
                className={styles.navButton}
                onClick={onNext}
                aria-label="Next session"
                title="Next session (Shift+→)"
              >
                →
              </button>
            </>
          )}
          <button
            className={styles.closeButton}
            onClick={onClose}
            aria-label="Close"
          >
            ✕
          </button>
        </div>
      </div>

      <div className={styles.tabs}>
        {tabs.map((tab) => (
          <button
            key={tab.id}
            className={`${styles.tab} ${activeTab === tab.id ? styles.active : ""}`}
            onClick={() => handleTabChange(tab.id)}
          >
            <span className={styles.tabIcon}>{tab.icon}</span>
            <span className={styles.tabLabel}>{tab.label}</span>
          </button>
        ))}
      </div>

      <div className={`${styles.content} ${isFullscreen ? styles.fullscreenContent : ""}`}>
        {activeTab === "terminal" && (
          <div className={styles.tabContent}>
            <ApprovalPanel sessionId={session.id} onResolved={onApprovalResolved} />
            {session.instanceType === InstanceType.EXTERNAL && !session.externalMetadata?.muxSocketPath ? (
              <div className={styles.noTerminalPlaceholder}>
                <span className={styles.noTerminalIcon}>⛓️</span>
                <p className={styles.noTerminalText}>Terminal not available</p>
                <p className={styles.noTerminalSubtext}>
                  This session is running in an external terminal. Use Approve / Deny above to respond to pending requests.
                </p>
              </div>
            ) : session.instanceType === InstanceType.EXTERNAL && session.externalMetadata?.muxSocketPath ? (
              <TerminalOutput
                sessionId={session.externalMetadata.muxSocketPath}
                baseUrl={getApiBaseUrl()}
                isExternal={true}
                tmuxSessionName={session.externalMetadata?.tmuxSessionName}
              />
            ) : (
              <TerminalOutput sessionId={session.id} baseUrl={getApiBaseUrl()} />
            )}
          </div>
        )}
        {activeTab === "diff" && (
          <div className={styles.tabContent}>
            <DiffViewer sessionId={session.id} baseUrl={getApiBaseUrl()} />
          </div>
        )}
        {activeTab === "vcs" && (
          <div className={styles.tabContent}>
            <VcsPanel sessionId={session.id} baseUrl={getApiBaseUrl()} />
          </div>
        )}
        {activeTab === "logs" && (
          <div className={styles.tabContent}>
            <SessionLogsTab sessionId={session.id} baseUrl={getApiBaseUrl()} />
          </div>
        )}
        {activeTab === "info" && (
          <div className={styles.tabContent}>
            <div className={styles.infoGrid}>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Session ID:</span>
                <span className={styles.infoValue}>{session.id}</span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Status:</span>
                <span className={styles.infoValue}>{session.status}</span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Branch:</span>
                <span className={styles.infoValue}>{session.branch}</span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Category:</span>
                <span className={styles.infoValue}>{session.category}</span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Created:</span>
                <span className={styles.infoValue}>
                  {session.createdAt ? new Date(Number(session.createdAt.seconds) * 1000).toLocaleString() : "N/A"}
                </span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Updated:</span>
                <span className={styles.infoValue}>
                  {session.updatedAt ? new Date(Number(session.updatedAt.seconds) * 1000).toLocaleString() : "N/A"}
                </span>
              </div>
              {session.path && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Workspace Path:</span>
                  <span className={styles.infoValue}>{session.path}</span>
                </div>
              )}
              {session.workingDir && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Working Directory:</span>
                  <span className={styles.infoValue}>{session.workingDir}</span>
                </div>
              )}
              {session.program && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Program:</span>
                  {isEditingProgram ? (
                    <div className={styles.editContainer}>
                      <select
                        value={programValue}
                        onChange={(e) => setProgramValue(e.target.value)}
                        autoFocus
                        className={styles.editInput}
                      >
                        {PROGRAMS.map((p) => (
                          <option key={p.value} value={p.value}>{p.label}</option>
                        ))}
                        {!isKnownProgram(programValue) && (
                          <option value={programValue}>Custom: {programValue}</option>
                        )}
                      </select>
                      <button onClick={handleSaveProgram} className={styles.saveButton}>
                        ✓
                      </button>
                      <button onClick={handleCancelProgramEdit} className={styles.cancelButton}>
                        ✕
                      </button>
                    </div>
                  ) : (
                    <span className={styles.infoValue}>
                      {getProgramDisplay(session.program)}
                      <button
                        onClick={() => setIsEditingProgram(true)}
                        className={styles.editButton}
                        title="Edit program"
                      >
                        ✏️
                      </button>
                    </span>
                  )}
                </div>
              )}
              {session.instanceType === InstanceType.EXTERNAL && (
                <>
                  <div className={styles.infoItem}>
                    <span className={styles.infoLabel}>Session Type:</span>
                    <span className={styles.infoValue}>External</span>
                  </div>
                  {session.externalMetadata?.sourceTerminal && (
                    <div className={styles.infoItem}>
                      <span className={styles.infoLabel}>Source:</span>
                      <span className={styles.infoValue}>{session.externalMetadata.sourceTerminal}</span>
                    </div>
                  )}
                  {session.externalMetadata?.muxEnabled && (
                    <div className={styles.infoItem}>
                      <span className={styles.infoLabel}>Mux Enabled:</span>
                      <span className={styles.infoValue}>✓ Yes</span>
                    </div>
                  )}
                  {session.externalMetadata?.tmuxSessionName && (
                    <div className={styles.infoItem}>
                      <span className={styles.infoLabel}>Tmux Session:</span>
                      <span className={styles.infoValue}>{session.externalMetadata.tmuxSessionName}</span>
                    </div>
                  )}
                </>
              )}
              {session.prompt && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Initial Prompt:</span>
                  <span className={styles.infoValue}>{session.prompt}</span>
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Workspace Switch Modal */}
      {showWorkspaceSwitchModal && (
        <WorkspaceSwitchModal
          sessionId={session.id}
          sessionName={session.title}
          baseUrl={getApiBaseUrl()}
          onClose={() => setShowWorkspaceSwitchModal(false)}
          onSwitched={() => {
            // The session will be updated via the event bus
            setShowWorkspaceSwitchModal(false);
          }}
        />
      )}
    </div>
  );
}
