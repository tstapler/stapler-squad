"use client";

import { useState, useEffect } from "react";
import type { LucideIcon } from "lucide-react";
import { Terminal, GitCompare, GitBranch, FolderOpen, ScrollText, Info } from "lucide-react";
import dynamic from "next/dynamic";
import { Session, InstanceType, SessionStatus, SessionType } from "@/gen/session/v1/types_pb";
import { DiffViewer } from "./DiffViewer";
import { VcsPanel } from "./VcsPanel";
import { WorkspaceSwitchModal } from "./WorkspaceSwitchModal";
import { SessionLogsTab } from "./SessionLogsTab";
import { FilesTab } from "./FilesTab";
import { ActionBar } from "@/components/ui/ActionBar";
import { useSessionActions } from "@/lib/hooks/useSessionActions";
import { getApiBaseUrl } from "@/lib/config";
import { getProgramDisplay, isKnownProgram, PROGRAMS } from "@/lib/constants/programs";
import { Modal, ModalContent, ModalTitle, ModalFooter } from "@/components/ui/Modal";
import { ResumeSessionModal } from "./ResumeSessionModal";
import { TagEditor } from "./TagEditor";
import * as styles from "./SessionDetail.css";
import type { SessionDetailTab } from "./SessionDetail";

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

export interface SessionDetailViewProps {
  session: Session;
  allSessions: Session[];
  actions: ReturnType<typeof useSessionActions>;
  onClose: () => void;
  onFullscreenChange?: (isFullscreen: boolean) => void;
  onTabChange?: (tab: SessionDetailTab) => void;
  initialTab?: SessionDetailTab;
  /** When true, hides the title header and tab strip — caller (e.g. PaneHeader) owns those. */
  embedded?: boolean;
  onNext?: () => void;
  onPrevious?: () => void;
  showNavigation?: boolean;
  onApprovalResolved?: () => void;
  onDismissFromQueue?: () => void;
  queuePosition?: number;
  queueTotal?: number;
}

function getStatusLabel(status: SessionStatus): string {
  switch (status) {
    case SessionStatus.RUNNING: return "Running";
    case SessionStatus.READY: return "Ready";
    case SessionStatus.LOADING: return "Loading";
    case SessionStatus.PAUSED: return "Paused";
    case SessionStatus.NEEDS_APPROVAL: return "Needs Approval";
    case SessionStatus.CREATING: return "Creating";
    case SessionStatus.STOPPED: return "Stopped";
    default: return "Unknown";
  }
}

function getSessionTypeLabel(type: SessionType): string {
  switch (type) {
    case SessionType.DIRECTORY: return "Directory";
    case SessionType.NEW_WORKTREE: return "New Worktree";
    case SessionType.EXISTING_WORKTREE: return "Existing Worktree";
    default: return "Unknown";
  }
}

const POOL_PANE_BASE: React.CSSProperties = {
  position: "absolute",
  top: 0, left: 0, right: 0, bottom: 0,
};

export function SessionDetailView({
  session,
  allSessions,
  actions,
  onClose,
  onFullscreenChange,
  onTabChange,
  initialTab = "info",
  embedded = false,
  onNext,
  onPrevious,
  showNavigation = false,
  onApprovalResolved: _onApprovalResolved,
  onDismissFromQueue,
  queuePosition,
  queueTotal,
}: SessionDetailViewProps) {
  const [activeTab, setActiveTab] = useState<SessionDetailTab>(initialTab);

  // Sync active tab when the pane's controlled tab changes (e.g. PaneHeader tab click)
  useEffect(() => {
    setActiveTab(initialTab);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialTab]);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [copiedField, setCopiedField] = useState<string | null>(null);
  const [filesSelectedPath, setFilesSelectedPath] = useState<string | null>(null);
  const [showWorkspaceSwitchModal, setShowWorkspaceSwitchModal] = useState(false);
  const [isEditingProgram, setIsEditingProgram] = useState(false);
  const [programValue, setProgramValue] = useState(session.program || "");
  const [isEditingWorkingDir, setIsEditingWorkingDir] = useState(false);
  const [workingDirValue, setWorkingDirValue] = useState(session.workingDir || "");
  // Action sheet state
  const [actionSheetOpen, setActionSheetOpen] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showResumeModal, setShowResumeModal] = useState(false);
  // Rename state
  const [showRenameModal, setShowRenameModal] = useState(false);
  const [renameValue, setRenameValue] = useState(session.title);
  const [renameError, setRenameError] = useState<string | null>(null);
  // Tag editor state
  const [showTagEditor, setShowTagEditor] = useState(false);
  const [showRestartConfirm, setShowRestartConfirm] = useState(false);
  const [showCheckpointModal, setShowCheckpointModal] = useState(false);
  const [checkpointLabel, setCheckpointLabel] = useState("");
  // Category/Tags inline editing state
  const [isEditingCategory, setIsEditingCategory] = useState(false);
  const [categoryValue, setCategoryValue] = useState(session.category || "");
  const [isEditingTags, setIsEditingTags] = useState(false);
  const [tagsInputValue, setTagsInputValue] = useState((session.tags ?? []).join(", "));
  // Launch command display state
  const [launchCmdExpanded, setLaunchCmdExpanded] = useState(false);
  const [copiedLaunchCmd, setCopiedLaunchCmd] = useState(false);

  // Measure click-to-render latency when the detail panel first mounts for a session
  useEffect(() => {
    if (typeof performance === "undefined") return;
    try {
      performance.measure("session:click-to-render", "session:click");
    } catch {
      // mark may be absent (e.g. deep-link navigation with no prior click)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session.id]);

  // Terminal instance pool: keeps up to 8 session terminals alive (LRU, oldest first)
  const [pooledSessionIds, setPooledSessionIds] = useState<string[]>([]);
  const [pooledMuxPaths, setPooledMuxPaths] = useState<string[]>([]);

  useEffect(() => {
    setPooledSessionIds(prev => {
      if (prev.includes(session.id)) return prev;
      const updated = [...prev, session.id];
      if (updated.length > 8) return updated.slice(-8);
      return updated;
    });
  }, [session.id]);

  useEffect(() => {
    const muxPath = session.externalMetadata?.muxSocketPath;
    if (!muxPath) return;
    setPooledMuxPaths(prev => {
      if (prev.includes(muxPath)) return prev;
      const updated = [...prev, muxPath];
      if (updated.length > 8) return updated.slice(-8);
      return updated;
    });
  }, [session.externalMetadata?.muxSocketPath]);

  // Notify parent of fullscreen state changes
  useEffect(() => {
    onFullscreenChange?.(isFullscreen);
  }, [isFullscreen, onFullscreenChange]);

  const tabs: { id: SessionDetailTab; label: string; icon: LucideIcon }[] = [
    { id: "terminal", label: "Terminal", icon: Terminal },
    { id: "diff", label: "Diff", icon: GitCompare },
    { id: "vcs", label: "VCS", icon: GitBranch },
    { id: "files", label: "Files", icon: FolderOpen },
    { id: "logs", label: "Logs", icon: ScrollText },
    { id: "info", label: "Info", icon: Info },
  ];

  const handleTabChange = (tabId: SessionDetailTab) => {
    setActiveTab(tabId);
    onTabChange?.(tabId);
  };

  const handleCopy = (field: string, value: string) => {
    navigator.clipboard.writeText(value)
      .then(() => {
        setCopiedField(field);
        setTimeout(() => setCopiedField(null), 1500);
      })
      .catch((err) => {
        console.warn("[SessionDetailView] clipboard write failed", err);
      });
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

  function makeStringFieldEditor(
    editValue: string,
    originalValue: string,
    setValue: (v: string) => void,
    setEditing: (v: boolean) => void,
    updateFn: (v: string) => Promise<unknown>,
  ) {
    return {
      handleSave: async () => {
        if (editValue !== originalValue) await updateFn(editValue);
        setEditing(false);
      },
      handleCancel: () => {
        setValue(originalValue);
        setEditing(false);
      },
    };
  }

  const { handleSave: handleSaveProgram, handleCancel: handleCancelProgramEdit } =
    makeStringFieldEditor(
      programValue, session.program || "", setProgramValue, setIsEditingProgram,
      (v) => actions.update({ program: v }),
    );

  const { handleSave: handleSaveWorkingDir, handleCancel: handleCancelWorkingDirEdit } =
    makeStringFieldEditor(
      workingDirValue, session.workingDir || "", setWorkingDirValue, setIsEditingWorkingDir,
      (v) => actions.update({ workingDir: v }),
    );

  const { handleSave: handleSaveCategory, handleCancel: handleCancelCategoryEdit } =
    makeStringFieldEditor(
      categoryValue, session.category || "", setCategoryValue, setIsEditingCategory,
      (v) => actions.update({ category: v }),
    );

  const handleSaveTags = async () => {
    const newTags = tagsInputValue.split(",").map((t) => t.trim()).filter(Boolean);
    const current = session.tags ?? [];
    if (JSON.stringify(newTags) !== JSON.stringify(current)) {
      await actions.update({ tags: newTags });
    }
    setIsEditingTags(false);
  };

  const handleCancelTagsEdit = () => {
    setTagsInputValue((session.tags ?? []).join(", "));
    setIsEditingTags(false);
  };

  // Action sheet handlers
  const handlePauseResume = async () => {
    if (session.status === SessionStatus.PAUSED) {
      setActionSheetOpen(false);
      setShowResumeModal(true);
    } else {
      await actions.pause();
      setActionSheetOpen(false);
    }
  };

  const handleDeleteClick = () => {
    if (session.status === SessionStatus.RUNNING || session.status === SessionStatus.NEEDS_APPROVAL) {
      setShowDeleteConfirm(true);
    } else {
      handleConfirmDelete();
    }
  };

  const handleConfirmDelete = async () => {
    setShowDeleteConfirm(false);
    setActionSheetOpen(false);
    await actions.delete();
    onClose();
  };

  const handleRenameSave = async () => {
    const trimmed = renameValue.trim();
    if (!trimmed) {
      setRenameError("Session name cannot be empty");
      return;
    }
    setRenameError(null);
    await actions.rename(trimmed);
    setShowRenameModal(false);
  };

  return (
    <div className={`${styles.container} ${isFullscreen ? styles.fullscreen : ""}`}>
      {!embedded && <div className={`${styles.header} ${isFullscreen ? styles.fullscreenMobileHeader : ""}`} data-testid="session-header">
        <h2
          className={`${styles.title} ${isFullscreen ? styles.fullscreenMobileTitle : ""}`}
          title={session.title}
          data-testid="session-header-title"
        >
          {session.title}
          <span className={styles.statusBadge} data-testid="session-status-badge">
            {getStatusLabel(session.status)}
          </span>
        </h2>
        <ActionBar gap="sm" justify="end" scroll className={`${styles.headerActions} ${isFullscreen ? styles.fullscreenMobileHeaderActions : ""}`}>
          {/* Fullscreen — most used when viewing terminal/diff/vcs */}
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
          {/* Queue navigation — most used in review queue mode */}
          {showNavigation && (
            <>
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
              {queuePosition !== undefined && queuePosition > 0 && queueTotal !== undefined && queueTotal > 0 && (
                <span className={styles.queuePosition} aria-live="polite">
                  {queuePosition} of {queueTotal}
                </span>
              )}
            </>
          )}
          {/* Dismiss from queue */}
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
          {/* Switch workspace — less frequent */}
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
          {/* More actions — opens action sheet */}
          <button
            className={styles.moreActionsButton}
            onClick={() => setActionSheetOpen(true)}
            aria-label="Session actions"
            data-testid="more-actions-button"
          >
            ⋯
          </button>
          {/* Close — conventional rightmost */}
          <button
            className={styles.closeButton}
            onClick={onClose}
            aria-label="Close"
          >
            ✕
          </button>
        </ActionBar>
      </div>}

      {!embedded && <div
        className={`${styles.tabs} ${isFullscreen ? styles.fullscreenMobileTabs : ""}`}
        role="tablist"
        onKeyDown={(e) => {
          const currentIndex = tabs.findIndex((t) => t.id === activeTab);
          if (e.key === "ArrowRight") {
            e.preventDefault();
            const nextIndex = (currentIndex + 1) % tabs.length;
            handleTabChange(tabs[nextIndex].id);
            (e.currentTarget.querySelectorAll('[role="tab"]')[nextIndex] as HTMLElement)?.focus();
          } else if (e.key === "ArrowLeft") {
            e.preventDefault();
            const prevIndex = (currentIndex - 1 + tabs.length) % tabs.length;
            handleTabChange(tabs[prevIndex].id);
            (e.currentTarget.querySelectorAll('[role="tab"]')[prevIndex] as HTMLElement)?.focus();
          }
        }}
      >
        {tabs.map((tab) => {
          const Icon = tab.icon;
          return (
            <button
              key={tab.id}
              id={`tab-${tab.id}`}
              role="tab"
              aria-selected={activeTab === tab.id}
              className={`${styles.tab} ${activeTab === tab.id ? styles.active : ""}`}
              onClick={() => handleTabChange(tab.id)}
            >
              <span className={styles.tabIcon}><Icon size={16} /></span>
              <span className={styles.tabLabel}>{tab.label}</span>
            </button>
          );
        })}
      </div>}

      <div className={`${styles.content} ${isFullscreen ? styles.fullscreenContent : ""}`}>
        {/* Terminal tab: kept mounted but hidden via display:none to preserve xterm.js instances */}
        <div
          className={styles.tabContent}
          role="tabpanel"
          aria-labelledby="tab-terminal"
          aria-hidden={activeTab !== "terminal"}
          style={{ display: activeTab === "terminal" ? undefined : 'none' }}
        >
          {/* ApprovalPanel removed — approvals now handled in the global ApprovalDrawer in Header */}
          {session.instanceType === InstanceType.EXTERNAL && !session.externalMetadata?.muxSocketPath ? (
            <div className={styles.noTerminalPlaceholder}>
              <span className={styles.noTerminalIcon}>⛓️</span>
              <p className={styles.noTerminalText}>Terminal not available</p>
              <p className={styles.noTerminalSubtext}>
                This session is running in an external terminal. Use Approve / Deny above to respond to pending requests.
              </p>
            </div>
          ) : session.instanceType === InstanceType.EXTERNAL && session.externalMetadata?.muxSocketPath ? (
            <div style={{ position: 'relative', flex: 1, minHeight: 0 }}>
              {pooledMuxPaths.map(poolPath => (
                <div
                  key={poolPath}
                  style={{
                    ...POOL_PANE_BASE,
                    visibility: poolPath === session.externalMetadata?.muxSocketPath ? "visible" : "hidden",
                    pointerEvents: poolPath === session.externalMetadata?.muxSocketPath ? "auto" : "none",
                  }}
                >
                  <TerminalOutput
                    sessionId={poolPath}
                    baseUrl={getApiBaseUrl()}
                    isExternal={true}
                    tmuxSessionName={session.externalMetadata?.tmuxSessionName}
                    isVisible={poolPath === session.externalMetadata?.muxSocketPath}
                  />
                </div>
              ))}
            </div>
          ) : (
            <div style={{ position: 'relative', flex: 1, minHeight: 0 }}>
              {pooledSessionIds.map(poolId => (
                <div
                  key={poolId}
                  style={{
                    ...POOL_PANE_BASE,
                    visibility: poolId === session.id ? "visible" : "hidden",
                    pointerEvents: poolId === session.id ? "auto" : "none",
                  }}
                >
                  <TerminalOutput
                    sessionId={poolId}
                    baseUrl={getApiBaseUrl()}
                    isVisible={poolId === session.id}
                  />
                </div>
              ))}
            </div>
          )}
        </div>
        {activeTab === "diff" && (
          <div className={styles.tabContent} role="tabpanel" aria-labelledby="tab-diff">
            <DiffViewer />
          </div>
        )}
        {activeTab === "vcs" && (
          <div className={styles.tabContent} role="tabpanel" aria-labelledby="tab-vcs">
            <VcsPanel
              session={session}
              onNavigateToFile={(path) => {
                setFilesSelectedPath(path);
                handleTabChange("files");
              }}
            />
          </div>
        )}
        {activeTab === "files" && (
          <div className={styles.tabContent} role="tabpanel" aria-labelledby="tab-files">
            <FilesTab
              sessionId={session.id}
              baseUrl={getApiBaseUrl()}
              initialSelectedPath={filesSelectedPath}
              onSelectedPathChange={setFilesSelectedPath}
            />
          </div>
        )}
        {activeTab === "logs" && (
          <div className={styles.tabContent} role="tabpanel" aria-labelledby="tab-logs">
            <SessionLogsTab sessionId={session.id} />
          </div>
        )}
        {activeTab === "info" && (
          <div className={styles.tabContent} role="tabpanel" aria-labelledby="tab-info">
            <div className={styles.infoGrid}>
              {/* Identity */}
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Instance ID:</span>
                <span className={styles.infoValue} style={{ fontFamily: 'monospace', fontSize: '0.9em' }}>
                  {session.id}
                  <button onClick={() => handleCopy('instanceId', session.id)} className={styles.editButton} title="Copy to clipboard">{copiedField === 'instanceId' ? '✓' : '📋'}</button>
                </span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Status:</span>
                <span className={styles.infoValue}>{getStatusLabel(session.status)}</span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Session Type:</span>
                <span className={styles.infoValue}>{getSessionTypeLabel(session.sessionType)}</span>
              </div>
              {session.instanceType === InstanceType.EXTERNAL && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Instance Type:</span>
                  <span className={styles.infoValue}>External</span>
                </div>
              )}
              {/* Timestamps */}
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
              {/* Location */}
              {session.branch && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Branch:</span>
                  <span className={styles.infoValue} style={{ fontFamily: 'monospace' }}>{session.branch}</span>
                </div>
              )}
              {session.path && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Workspace Path:</span>
                  <span className={styles.infoValue} style={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>{session.path}</span>
                </div>
              )}
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Working Directory:</span>
                {isEditingWorkingDir ? (
                  <div className={styles.editContainer}>
                    <input
                      type="text"
                      value={workingDirValue}
                      onChange={(e) => setWorkingDirValue(e.target.value)}
                      onKeyDown={(e) => { if (e.key === "Enter") handleSaveWorkingDir(); else if (e.key === "Escape") handleCancelWorkingDirEdit(); }}
                      autoFocus
                      className={styles.editInput}
                      placeholder={session.path || ""}
                      style={{ fontFamily: 'monospace', minWidth: '20ch' }}
                    />
                    <button onClick={handleSaveWorkingDir} className={styles.saveButton}>✓</button>
                    <button onClick={handleCancelWorkingDirEdit} className={styles.cancelButton}>✕</button>
                  </div>
                ) : (
                  <span className={styles.infoValue} style={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>
                    {session.workingDir || <em style={{ opacity: 0.5 }}>{session.path}</em>}
                    <button
                      onClick={() => { setWorkingDirValue(session.workingDir || ""); setIsEditingWorkingDir(true); }}
                      className={styles.editButton}
                      title="Edit working directory"
                    >
                      ✏️
                    </button>
                  </span>
                )}
              </div>
              {/* Organization */}
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Category:</span>
                {isEditingCategory ? (
                  <div className={styles.editContainer}>
                    <input
                      type="text"
                      value={categoryValue}
                      onChange={(e) => setCategoryValue(e.target.value)}
                      onKeyDown={(e) => { if (e.key === "Enter") handleSaveCategory(); else if (e.key === "Escape") handleCancelCategoryEdit(); }}
                      autoFocus
                      className={styles.editInput}
                      placeholder="e.g. Work/Frontend"
                    />
                    <button onClick={handleSaveCategory} className={styles.saveButton}>✓</button>
                    <button onClick={handleCancelCategoryEdit} className={styles.cancelButton}>✕</button>
                  </div>
                ) : (
                  <span className={styles.infoValue}>
                    {session.category || <em style={{ opacity: 0.5 }}>None</em>}
                    <button
                      onClick={() => { setCategoryValue(session.category || ""); setIsEditingCategory(true); }}
                      className={styles.editButton}
                      title="Edit category"
                    >
                      ✏️
                    </button>
                  </span>
                )}
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Tags:</span>
                {isEditingTags ? (
                  <div className={styles.editContainer}>
                    <input
                      type="text"
                      value={tagsInputValue}
                      onChange={(e) => setTagsInputValue(e.target.value)}
                      onKeyDown={(e) => { if (e.key === "Enter") handleSaveTags(); else if (e.key === "Escape") handleCancelTagsEdit(); }}
                      autoFocus
                      className={styles.editInput}
                      placeholder="tag1, tag2, tag3"
                    />
                    <button onClick={handleSaveTags} className={styles.saveButton}>✓</button>
                    <button onClick={handleCancelTagsEdit} className={styles.cancelButton}>✕</button>
                  </div>
                ) : (
                  <span className={styles.infoValue}>
                    {(session.tags ?? []).length > 0 ? session.tags!.join(", ") : <em style={{ opacity: 0.5 }}>None</em>}
                    <button
                      onClick={() => { setTagsInputValue((session.tags ?? []).join(", ")); setIsEditingTags(true); }}
                      className={styles.editButton}
                      title="Edit tags"
                    >
                      ✏️
                    </button>
                  </span>
                )}
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Auto Yes:</span>
                <span className={styles.infoValue}>{session.autoYes ? "Enabled" : "Disabled"}</span>
              </div>
              {/* Program */}
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
                    <button onClick={handleSaveProgram} className={styles.saveButton}>✓</button>
                    <button onClick={handleCancelProgramEdit} className={styles.cancelButton}>✕</button>
                  </div>
                ) : (
                  <span className={styles.infoValue}>
                    {session.program ? getProgramDisplay(session.program) : <em style={{ opacity: 0.5 }}>None</em>}
                    <button
                      onClick={() => { setProgramValue(session.program || ""); setIsEditingProgram(true); }}
                      className={styles.editButton}
                      title="Edit program"
                    >
                      ✏️
                    </button>
                  </span>
                )}
              </div>
              <div className={styles.infoItem} style={{ alignItems: 'flex-start' }}>
                <span className={styles.infoLabel} style={{ paddingTop: '2px' }}>Launch Command:</span>
                <div style={{ flex: 1, minWidth: 0 }}>
                  {session.launchCommand ? (
                    <>
                      <pre style={{
                        fontFamily: 'monospace',
                        fontSize: '0.82em',
                        margin: 0,
                        padding: '6px 8px',
                        background: 'var(--terminal-background, #1a1a2e)',
                        color: 'var(--terminal-foreground, #e0e0e0)',
                        borderRadius: '4px',
                        overflowX: launchCmdExpanded ? 'auto' : 'hidden',
                        whiteSpace: launchCmdExpanded ? 'pre' : 'nowrap',
                        textOverflow: launchCmdExpanded ? 'clip' : 'ellipsis',
                        maxHeight: launchCmdExpanded ? '200px' : undefined,
                        overflowY: launchCmdExpanded ? 'auto' : undefined,
                        cursor: 'default',
                      }}>
                        {session.launchCommand}
                      </pre>
                      <div style={{ display: 'flex', gap: '8px', marginTop: '4px' }}>
                        <button
                          onClick={() => setLaunchCmdExpanded((v) => !v)}
                          className={styles.editButton}
                          title={launchCmdExpanded ? "Collapse" : "Expand full command"}
                          style={{ fontSize: '0.75em' }}
                        >
                          {launchCmdExpanded ? "▲ collapse" : "▼ expand"}
                        </button>
                        <button
                          onClick={() => {
                            navigator.clipboard.writeText(session.launchCommand)
                              .then(() => {
                                setCopiedLaunchCmd(true);
                                setTimeout(() => setCopiedLaunchCmd(false), 2000);
                              })
                              .catch((err) => {
                                console.warn("[SessionDetailView] clipboard write failed", err);
                              });
                          }}
                          className={styles.editButton}
                          title="Copy to clipboard"
                          style={{ fontSize: '0.75em' }}
                        >
                          {copiedLaunchCmd ? "✓ copied" : "⎘ copy"}
                        </button>
                      </div>
                    </>
                  ) : (
                    <em style={{ opacity: 0.5 }}>Not started yet</em>
                  )}
                </div>
              </div>
              {session.tmuxPrefix && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Tmux Prefix:</span>
                  <span className={styles.infoValue} style={{ fontFamily: 'monospace' }}>{session.tmuxPrefix}</span>
                </div>
              )}
              {/* Claude session */}
              {session.claudeSession?.sessionId && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Claude Conversation UUID:</span>
                  <span className={styles.infoValue} style={{ fontFamily: 'monospace', fontSize: '0.9em' }}>
                    {session.claudeSession.sessionId}
                    <button onClick={() => handleCopy('claudeUuid', session.claudeSession!.sessionId)} className={styles.editButton} title="Copy to clipboard">{copiedField === 'claudeUuid' ? '✓' : '📋'}</button>
                  </span>
                </div>
              )}
              {session.claudeSession?.projectName && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Claude Project:</span>
                  <span className={styles.infoValue}>{session.claudeSession.projectName}</span>
                </div>
              )}
              {session.historyFilePath && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>History File:</span>
                  <span className={styles.infoValue} style={{ fontFamily: 'monospace', wordBreak: 'break-all', fontSize: '0.85em' }}>{session.historyFilePath}</span>
                </div>
              )}
              {/* Git worktree */}
              {session.gitWorktree && (
                <>
                  {session.gitWorktree.repoPath && (
                    <div className={styles.infoItem}>
                      <span className={styles.infoLabel}>Repo Path:</span>
                      <span className={styles.infoValue} style={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>{session.gitWorktree.repoPath}</span>
                    </div>
                  )}
                  {session.gitWorktree.worktreePath && (
                    <div className={styles.infoItem}>
                      <span className={styles.infoLabel}>Worktree Path:</span>
                      <span className={styles.infoValue} style={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>{session.gitWorktree.worktreePath}</span>
                    </div>
                  )}
                  {session.gitWorktree.branchName && (
                    <div className={styles.infoItem}>
                      <span className={styles.infoLabel}>Worktree Branch:</span>
                      <span className={styles.infoValue} style={{ fontFamily: 'monospace' }}>{session.gitWorktree.branchName}</span>
                    </div>
                  )}
                  {session.gitWorktree.baseCommitSha && (
                    <div className={styles.infoItem}>
                      <span className={styles.infoLabel}>Base Commit:</span>
                      <span className={styles.infoValue} style={{ fontFamily: 'monospace', fontSize: '0.85em' }}>
                        {session.gitWorktree.baseCommitSha}
                        <button onClick={() => handleCopy('baseCommit', session.gitWorktree!.baseCommitSha)} className={styles.editButton} title="Copy to clipboard">{copiedField === 'baseCommit' ? '✓' : '📋'}</button>
                      </span>
                    </div>
                  )}
                </>
              )}
              {/* Diff stats */}
              {session.diffStats && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Diff Stats:</span>
                  <span className={styles.infoValue}>
                    <span style={{ color: 'var(--color-success, #22c55e)' }}>+{session.diffStats.added}</span>
                    {" / "}
                    <span style={{ color: 'var(--color-error, #ef4444)' }}>-{session.diffStats.removed}</span>
                  </span>
                </div>
              )}
              {/* GitHub PR */}
              {session.githubPrUrl && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>GitHub PR:</span>
                  <span className={styles.infoValue}>
                    <a href={session.githubPrUrl} target="_blank" rel="noopener noreferrer" style={{ color: 'var(--color-link, #3b82f6)' }}>
                      #{session.githubPrNumber} {session.githubPrState && `(${session.githubPrState})`}
                      {session.githubPrIsDraft && " [Draft]"}
                    </a>
                  </span>
                </div>
              )}
              {session.githubOwner && session.githubRepo && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>GitHub Repo:</span>
                  <span className={styles.infoValue} style={{ fontFamily: 'monospace' }}>{session.githubOwner}/{session.githubRepo}</span>
                </div>
              )}
              {session.githubSourceRef && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Source Ref:</span>
                  <span className={styles.infoValue} style={{ fontFamily: 'monospace' }}>{session.githubSourceRef}</span>
                </div>
              )}
              {(session.githubApprovedCount > 0 || session.githubChangesReqCount > 0) && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Reviews:</span>
                  <span className={styles.infoValue}>
                    {session.githubApprovedCount > 0 && <span style={{ color: 'var(--color-success, #22c55e)' }}>{session.githubApprovedCount} approved</span>}
                    {session.githubApprovedCount > 0 && session.githubChangesReqCount > 0 && " · "}
                    {session.githubChangesReqCount > 0 && <span style={{ color: 'var(--color-error, #ef4444)' }}>{session.githubChangesReqCount} changes requested</span>}
                  </span>
                </div>
              )}
              {session.githubCheckConclusion && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>CI Status:</span>
                  <span className={styles.infoValue}>{session.githubCheckConclusion}</span>
                </div>
              )}
              {session.clonedRepoPath && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Cloned Repo:</span>
                  <span className={styles.infoValue} style={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>{session.clonedRepoPath}</span>
                </div>
              )}
              {/* External session metadata */}
              {session.instanceType === InstanceType.EXTERNAL && session.externalMetadata && (
                <>
                  {session.externalMetadata.sourceTerminal && (
                    <div className={styles.infoItem}>
                      <span className={styles.infoLabel}>Source Terminal:</span>
                      <span className={styles.infoValue}>{session.externalMetadata.sourceTerminal}</span>
                    </div>
                  )}
                  {session.externalMetadata.muxEnabled && (
                    <div className={styles.infoItem}>
                      <span className={styles.infoLabel}>Mux Enabled:</span>
                      <span className={styles.infoValue}>Yes</span>
                    </div>
                  )}
                  {session.externalMetadata.tmuxSessionName && (
                    <div className={styles.infoItem}>
                      <span className={styles.infoLabel}>Tmux Session:</span>
                      <span className={styles.infoValue} style={{ fontFamily: 'monospace' }}>{session.externalMetadata.tmuxSessionName}</span>
                    </div>
                  )}
                </>
              )}
              {/* Initial prompt */}
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

      {/* Action Sheet — Radix Dialog with bottom-sheet behavior on mobile via globals.css */}
      <Modal open={actionSheetOpen} onOpenChange={setActionSheetOpen}>
        <ModalContent fallbackTitle={session.title} data-testid="action-sheet">
          <ModalTitle>{session.title}</ModalTitle>
          <div className={styles.actionSheet}>
            {/* Pause/Resume — hide for external sessions */}
            {session.instanceType !== InstanceType.EXTERNAL && (
              <button
                className={styles.actionSheetItem}
                onClick={handlePauseResume}
                data-testid="action-pause"
              >
                {session.status === SessionStatus.PAUSED ? '▶ Resume' : '⏸ Pause'}
              </button>
            )}
            <button
              className={styles.actionSheetItem}
              onClick={() => { setActionSheetOpen(false); setRenameValue(session.title); setShowRenameModal(true); }}
              data-testid="action-rename"
            >
              ✏️ Rename
            </button>
            <button
              className={styles.actionSheetItem}
              onClick={() => { setActionSheetOpen(false); setShowTagEditor(true); }}
              data-testid="action-edit-tags"
            >
              🏷 Edit Tags
            </button>
            {session.instanceType !== InstanceType.EXTERNAL && (
              <button
                className={styles.actionSheetItem}
                onClick={() => { setActionSheetOpen(false); setCheckpointLabel(""); setShowCheckpointModal(true); }}
                data-testid="action-checkpoint"
              >
                📸 Create Checkpoint
              </button>
            )}
            {/* Switch workspace */}
            {session.instanceType !== InstanceType.EXTERNAL && (
              <button
                className={styles.actionSheetItem}
                onClick={() => { setActionSheetOpen(false); setShowWorkspaceSwitchModal(true); }}
              >
                ⎇ Switch Workspace
              </button>
            )}
            {session.instanceType !== InstanceType.EXTERNAL && (
              <button
                className={styles.actionSheetItem}
                onClick={() => { setActionSheetOpen(false); setShowRestartConfirm(true); }}
                data-testid="action-restart"
              >
                🔄 Restart
              </button>
            )}
            <hr className={styles.actionDivider} />
            <button
              className={`${styles.actionSheetItem} ${styles.actionSheetItemDestructive}`}
              onClick={handleDeleteClick}
              data-testid="action-delete"
            >
              🗑 Delete
            </button>
          </div>
        </ModalContent>
      </Modal>

      {/* Delete confirmation dialog */}
      <Modal open={showDeleteConfirm} onOpenChange={setShowDeleteConfirm}>
        <ModalContent fallbackTitle="Confirm delete" data-testid="delete-confirm-dialog">
          <ModalTitle>Delete session?</ModalTitle>
          <p style={{ color: 'var(--text-secondary)', marginBottom: '1rem' }}>
            This session is currently running. Stop and delete it?
          </p>
          <ModalFooter>
            <button
              className={styles.actionButton}
              onClick={() => setShowDeleteConfirm(false)}
            >
              Cancel
            </button>
            <button
              className={`${styles.actionButton} ${styles.actionButtonDanger}`}
              onClick={handleConfirmDelete}
              data-testid="delete-confirm"
            >
              Delete
            </button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Rename modal */}
      <Modal open={showRenameModal} onOpenChange={setShowRenameModal}>
        <ModalContent fallbackTitle="Rename session">
          <ModalTitle>Rename session</ModalTitle>
          <input
            type="text"
            value={renameValue}
            onChange={(e) => { setRenameValue(e.target.value); setRenameError(null); }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleRenameSave();
              else if (e.key === 'Escape') setShowRenameModal(false);
            }}
            autoFocus
            className={styles.editInput}
            data-testid="rename-input"
          />
          {renameError && (
            <p style={{ color: 'var(--error)', fontSize: '0.875rem', marginTop: '0.25rem' }}>{renameError}</p>
          )}
          <ModalFooter>
            <button className={styles.actionButton} onClick={() => setShowRenameModal(false)}>
              Cancel
            </button>
            <button
              className={`${styles.actionButton} ${styles.actionButtonSave}`}
              onClick={handleRenameSave}
              data-testid="rename-save"
            >
              Save
            </button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Restart confirmation */}
      <Modal open={showRestartConfirm} onOpenChange={setShowRestartConfirm}>
        <ModalContent fallbackTitle="Restart session">
          <ModalTitle>Restart session?</ModalTitle>
          <p style={{ color: 'var(--text-secondary)', marginBottom: '1rem' }}>
            This will stop and restart the session process.
          </p>
          <ModalFooter>
            <button className={styles.actionButton} onClick={() => setShowRestartConfirm(false)}>
              Cancel
            </button>
            <button
              className={`${styles.actionButton} ${styles.actionButtonDanger}`}
              onClick={async () => { setShowRestartConfirm(false); await actions.restart(); }}
              data-testid="restart-confirm"
            >
              Restart
            </button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Create checkpoint */}
      <Modal open={showCheckpointModal} onOpenChange={setShowCheckpointModal}>
        <ModalContent fallbackTitle="Create checkpoint">
          <ModalTitle>Create checkpoint</ModalTitle>
          <input
            type="text"
            value={checkpointLabel}
            onChange={(e) => setCheckpointLabel(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') { setShowCheckpointModal(false); actions.createCheckpoint(checkpointLabel.trim()); }
              else if (e.key === 'Escape') setShowCheckpointModal(false);
            }}
            placeholder="Optional label..."
            autoFocus
            className={styles.editInput}
            data-testid="checkpoint-input"
          />
          <ModalFooter>
            <button className={styles.actionButton} onClick={() => setShowCheckpointModal(false)}>
              Cancel
            </button>
            <button
              className={`${styles.actionButton} ${styles.actionButtonSave}`}
              onClick={() => { setShowCheckpointModal(false); actions.createCheckpoint(checkpointLabel.trim()); }}
              data-testid="checkpoint-save"
            >
              Save
            </button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Tag editor */}
      {showTagEditor && (
        <TagEditor
          tags={session.tags || []}
          sessionTitle={session.title}
          onSave={(tags) => { actions.updateTags(tags); setShowTagEditor(false); }}
          onCancel={() => setShowTagEditor(false)}
        />
      )}

      {/* Resume session modal */}
      {showResumeModal && (
        <ResumeSessionModal
          session={session}
          sessions={allSessions}
          onConfirm={async (updates) => {
            await actions.resume({ title: updates.title, tags: updates.tags });
            setShowResumeModal(false);
          }}
          onCancel={() => setShowResumeModal(false)}
        />
      )}
    </div>
  );
}
