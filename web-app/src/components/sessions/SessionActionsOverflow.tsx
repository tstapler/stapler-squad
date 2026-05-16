"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import { createPortal } from "react-dom";
import { MoreHorizontal } from "lucide-react";
import type { Session, CheckpointProto } from "@/gen/session/v1/types_pb";
import { SessionStatus } from "@/gen/session/v1/types_pb";
import { TagEditor } from "./TagEditor";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import {
  desktopActions,
  overflowContainer,
  overflowButton,
  overflowMenu,
  overflowMenuItem,
  overflowMenuItemDanger,
  actionButton,
  confirmDialog,
  renameDialog,
  dialogContent,
  dialogActions,
  submitButton,
  cancelButton,
  dangerButton,
  warningText,
  renameInput,
  errorMessage,
} from "./SessionActionsOverflow.css";

export interface SessionActionsOverflowProps {
  session: Session;
  /** Show Resume/Pause as a shortcut button before the ··· */
  showPrimaryAction?: boolean;
  /** Override className on the ··· trigger button (e.g. for compact toolbar use) */
  buttonClassName?: string;
  onResume?: () => void;
  onPause?: () => void;
  onDelete?: () => Promise<void> | void;
  onRestart?: (sessionId: string) => Promise<boolean | void>;
  onClone?: () => void;
  onOpenInNewPane?: () => void;
  onNewWorkspace?: () => void;
  onCreateCheckpoint?: (sessionId: string, label: string) => Promise<boolean>;
  onRunOneShot?: (sessionId: string) => Promise<void>;
  onSetRateLimitEnabled?: (sessionId: string, enabled: boolean) => void;
  onClearConversationState?: (sessionId: string) => Promise<boolean>;
  onUpdateTags?: (sessionId: string, tags: string[]) => void;
  /** Trigger rename flow in parent (e.g. SessionDetail opens its rename modal) */
  onRenameRequest?: () => void;
  /** Trigger workspace switch in parent (SessionDetail-specific) */
  onWorkspaceSwitchRequest?: () => void;
}

export function SessionActionsOverflow({
  session,
  showPrimaryAction = false,
  buttonClassName,
  onResume,
  onPause,
  onDelete,
  onRestart,
  onClone,
  onOpenInNewPane,
  onNewWorkspace,
  onCreateCheckpoint,
  onRunOneShot,
  onSetRateLimitEnabled,
  onClearConversationState,
  onUpdateTags,
  onRenameRequest,
  onWorkspaceSwitchRequest,
}: SessionActionsOverflowProps) {
  const isPaused = session.status === SessionStatus.PAUSED;
  const isReady = session.status === SessionStatus.NEEDS_APPROVAL;
  const isRunning = session.status === SessionStatus.RUNNING;

  const [showOverflow, setShowOverflow] = useState(false);
  const [menuPos, setMenuPos] = useState({ top: 0, right: 0 });
  const [isRestartConfirmOpen, setIsRestartConfirmOpen] = useState(false);
  const [isRestarting, setIsRestarting] = useState(false);
  const [restartError, setRestartError] = useState("");
  const [isDeleteConfirmOpen, setIsDeleteConfirmOpen] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const [isCheckpointOpen, setIsCheckpointOpen] = useState(false);
  const [checkpointLabel, setCheckpointLabel] = useState("");
  const [isCreatingCheckpoint, setIsCreatingCheckpoint] = useState(false);
  const [checkpointError, setCheckpointError] = useState("");
  const [isTagEditorOpen, setIsTagEditorOpen] = useState(false);
  const [isRunningOneShot, setIsRunningOneShot] = useState(false);
  const [oneShotResult, setOneShotResult] = useState<string | null>(null);

  const overflowContainerRef = useRef<HTMLDivElement>(null);
  const overflowButtonRef = useRef<HTMLButtonElement>(null);
  const overflowMenuRef = useRef<HTMLDivElement>(null);
  const restartDialogRef = useRef<HTMLDivElement>(null);
  const deleteDialogRef = useRef<HTMLDivElement>(null);
  const checkpointDialogRef = useRef<HTMLDivElement>(null);
  const restartTriggerRef = useRef<HTMLButtonElement>(null);
  const checkpointTriggerRef = useRef<HTMLButtonElement>(null);

  useFocusTrap(overflowMenuRef, showOverflow);
  useFocusTrap(restartDialogRef, isRestartConfirmOpen, restartTriggerRef);
  useFocusTrap(deleteDialogRef, isDeleteConfirmOpen);
  useFocusTrap(checkpointDialogRef, isCheckpointOpen, checkpointTriggerRef);

  useEffect(() => {
    if (showOverflow && overflowMenuRef.current) {
      const first = overflowMenuRef.current.querySelector<HTMLElement>('[role="menuitem"]');
      first?.focus();
    }
  }, [showOverflow]);

  useEffect(() => {
    if (!showOverflow) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      const inContainer = overflowContainerRef.current?.contains(target);
      const inMenu = overflowMenuRef.current?.contains(target);
      if (!inContainer && !inMenu) {
        setShowOverflow(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [showOverflow]);

  const openMenu = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    if (!overflowButtonRef.current) return;
    const rect = overflowButtonRef.current.getBoundingClientRect();
    setMenuPos({
      top: rect.bottom + 4,
      right: window.innerWidth - rect.right,
    });
    setShowOverflow((o) => !o);
  }, []);

  const close = () => setShowOverflow(false);

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

  const handleRestartConfirm = async (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsRestarting(true);
    setRestartError("");
    try {
      await onRestart?.(session.id);
      setIsRestartConfirmOpen(false);
    } catch (err) {
      setRestartError(err instanceof Error ? err.message : "Failed to restart session.");
    } finally {
      setIsRestarting(false);
    }
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
    } catch (err) {
      setCheckpointError(err instanceof Error ? err.message : "Failed to create checkpoint");
    } finally {
      setIsCreatingCheckpoint(false);
    }
  };

  return (
    <>
      {isTagEditorOpen && onUpdateTags && (
        <TagEditor
          tags={session.tags || []}
          onSave={(newTags) => { onUpdateTags(session.id, newTags); setIsTagEditorOpen(false); }}
          onCancel={() => setIsTagEditorOpen(false)}
          sessionTitle={session.title}
        />
      )}

      {isRestartConfirmOpen && createPortal(
        <div className={confirmDialog} onClick={(e) => { e.stopPropagation(); setIsRestartConfirmOpen(false); }}>
          <div
            ref={restartDialogRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby="restartDialogTitle"
            className={dialogContent}
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => { if (e.key === "Escape") setIsRestartConfirmOpen(false); }}
          >
            <h3 id="restartDialogTitle">Restart Session</h3>
            <p>Are you sure you want to restart &quot;{session.title}&quot;?</p>
            <p className={warningText}>This will terminate the current process and start a new one.</p>
            {restartError && <p className={errorMessage}>{restartError}</p>}
            <div className={dialogActions}>
              <button onClick={handleRestartConfirm} disabled={isRestarting} className={dangerButton}>
                {isRestarting ? "Restarting..." : "Restart"}
              </button>
              <button onClick={(e) => { e.stopPropagation(); setIsRestartConfirmOpen(false); setRestartError(""); }} disabled={isRestarting} className={cancelButton}>
                Cancel
              </button>
            </div>
          </div>
        </div>,
        document.body
      )}

      {isDeleteConfirmOpen && createPortal(
        <div className={confirmDialog} onClick={(e) => { e.stopPropagation(); setIsDeleteConfirmOpen(false); }}>
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
            {deleteError && <p className={errorMessage}>{deleteError}</p>}
            <div className={dialogActions}>
              <button
                onClick={async (e) => {
                  e.stopPropagation();
                  setIsDeleting(true);
                  setDeleteError("");
                  try { await onDelete?.(); setIsDeleteConfirmOpen(false); } catch (err) {
                    setDeleteError(err instanceof Error ? err.message : "Failed to delete session.");
                  } finally { setIsDeleting(false); }
                }}
                disabled={isDeleting}
                className={dangerButton}
              >
                {isDeleting ? "Deleting..." : "Delete"}
              </button>
              <button onClick={(e) => { e.stopPropagation(); setIsDeleteConfirmOpen(false); setDeleteError(""); }} disabled={isDeleting} className={cancelButton}>
                Cancel
              </button>
            </div>
          </div>
        </div>,
        document.body
      )}

      {isCheckpointOpen && createPortal(
        <div className={renameDialog} onClick={(e) => { e.stopPropagation(); setIsCheckpointOpen(false); setCheckpointError(""); }}>
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
                if (e.key === "Escape") { setIsCheckpointOpen(false); setCheckpointError(""); }
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
                onClick={(e) => { e.stopPropagation(); setIsCheckpointOpen(false); setCheckpointError(""); }}
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

      <div className={desktopActions}>
        {showPrimaryAction && (isPaused || isReady) && (
          <button
            className={actionButton}
            onClick={(e) => { e.stopPropagation(); onResume?.(); }}
            aria-label={`Resume session ${session.title}`}
            title="Resume this session"
          >
            <span aria-hidden="true">▶️</span> Resume
          </button>
        )}
        {showPrimaryAction && isRunning && (
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
            ref={overflowButtonRef}
            id={`overflow-btn-${session.id}`}
            className={buttonClassName ?? overflowButton}
            onClick={openMenu}
            aria-label="More session actions"
            aria-expanded={showOverflow}
            aria-haspopup="menu"
            aria-controls={`overflow-menu-${session.id}`}
          >
            <MoreHorizontal size={16} />
          </button>
          {showOverflow && createPortal(
            <div
              ref={overflowMenuRef}
              id={`overflow-menu-${session.id}`}
              className={overflowMenu}
              style={{ top: menuPos.top, right: menuPos.right }}
              role="menu"
              aria-labelledby={`overflow-btn-${session.id}`}
              onClick={(e) => e.stopPropagation()}
              onKeyDown={(e) => { if (e.key === "Escape") setShowOverflow(false); }}
            >
              {!(isPaused || isReady) && onResume && (
                <button role="menuitem" className={overflowMenuItem}
                  onClick={(e) => { e.stopPropagation(); close(); onResume(); }}
                  aria-label={`Resume session ${session.title}`}
                >
                  <span aria-hidden="true">▶️</span> Resume
                </button>
              )}
              {!isRunning && onPause && (
                <button role="menuitem" className={overflowMenuItem}
                  onClick={(e) => { e.stopPropagation(); close(); onPause(); }}
                  aria-label={`Pause session ${session.title}`}
                >
                  <span aria-hidden="true">⏸️</span> Pause
                </button>
              )}
              {onRenameRequest && (
                <button role="menuitem" className={overflowMenuItem}
                  onClick={(e) => { e.stopPropagation(); close(); onRenameRequest(); }}
                  aria-label={`Rename session ${session.title}`}
                >
                  <span aria-hidden="true">✏️</span> Rename
                </button>
              )}
              {onRestart && (
                <button
                  ref={restartTriggerRef}
                  role="menuitem"
                  className={`${overflowMenuItem} ${overflowMenuItemDanger}`}
                  onClick={(e) => { e.stopPropagation(); close(); setIsRestartConfirmOpen(true); }}
                  aria-label={`Restart session ${session.title}`}
                >
                  <span aria-hidden="true">🔄</span> Restart
                </button>
              )}
              {onCreateCheckpoint && (
                <button
                  ref={checkpointTriggerRef}
                  role="menuitem"
                  className={overflowMenuItem}
                  onClick={(e) => { e.stopPropagation(); close(); setCheckpointLabel(""); setIsCheckpointOpen(true); }}
                  aria-label={`Create checkpoint for session ${session.title}`}
                >
                  <span aria-hidden="true">📍</span> Checkpoint
                </button>
              )}
              {onClone && (
                <button role="menuitem" className={overflowMenuItem}
                  onClick={(e) => { e.stopPropagation(); close(); onClone(); }}
                  aria-label={`Clone session ${session.title}`}
                >
                  <span aria-hidden="true">⊕</span> Clone
                </button>
              )}
              {onRunOneShot && (
                <button role="menuitem" className={overflowMenuItem}
                  onClick={(e) => { close(); handleRunOneShot(e); }}
                  disabled={isRunningOneShot}
                  aria-label={`Create PR for session ${session.title}`}
                >
                  <span aria-hidden="true">🚀</span>{" "}
                  {isRunningOneShot ? "Creating PR…" : oneShotResult === "done" ? "✅ PR Created" : oneShotResult === "error" ? "❌ Retry?" : "Create PR"}
                </button>
              )}
              {onOpenInNewPane && (
                <button role="menuitem" className={overflowMenuItem}
                  onClick={(e) => { e.stopPropagation(); close(); onOpenInNewPane(); }}
                  aria-label={`Open ${session.title} in new pane`}
                >
                  <span aria-hidden="true">⊞</span> Open in new pane
                </button>
              )}
              {onUpdateTags && (
                <button role="menuitem" className={overflowMenuItem}
                  onClick={(e) => { e.stopPropagation(); close(); setIsTagEditorOpen(true); }}
                  aria-label={`Edit tags for session ${session.title}`}
                >
                  <span aria-hidden="true">🏷️</span> Edit Tags
                </button>
              )}
              {onSetRateLimitEnabled && (
                <button role="menuitem" className={overflowMenuItem}
                  onClick={(e) => { e.stopPropagation(); close(); onSetRateLimitEnabled(session.id, !session.rateLimitEnabled); }}
                  aria-label={session.rateLimitEnabled ? `Disable auto-resume for ${session.title}` : `Enable auto-resume for ${session.title}`}
                >
                  <span aria-hidden="true">{session.rateLimitEnabled ? "⏸" : "▶"}</span>{" "}
                  {session.rateLimitEnabled ? "Disable auto-resume" : "Enable auto-resume"}
                </button>
              )}
              {onNewWorkspace && (
                <button role="menuitem" className={overflowMenuItem}
                  onClick={(e) => { e.stopPropagation(); close(); onNewWorkspace(); }}
                  aria-label={`New workspace from ${session.title}`}
                >
                  <span aria-hidden="true">➕</span> New Workspace
                </button>
              )}
              {onWorkspaceSwitchRequest && (
                <button role="menuitem" className={overflowMenuItem}
                  onClick={(e) => { e.stopPropagation(); close(); onWorkspaceSwitchRequest(); }}
                  aria-label={`Switch workspace for ${session.title}`}
                >
                  <span aria-hidden="true">⎇</span> Switch Workspace
                </button>
              )}
              {onClearConversationState && (
                <button role="menuitem" className={overflowMenuItem}
                  onClick={(e) => { e.stopPropagation(); close(); void onClearConversationState(session.id); }}
                  aria-label={`Clear conversation state for session ${session.title}`}
                >
                  <span aria-hidden="true">🗑️</span> Clear Conversation
                </button>
              )}
              {onDelete && (
                <button role="menuitem" className={`${overflowMenuItem} ${overflowMenuItemDanger}`}
                  onClick={(e) => { e.stopPropagation(); close(); setIsDeleteConfirmOpen(true); }}
                  disabled={isDeleting}
                  aria-label={`Delete session ${session.title}`}
                >
                  {isDeleting ? "Deleting..." : <><span aria-hidden="true">🗑️</span> Delete</>}
                </button>
              )}
            </div>,
            document.body
          )}
        </div>
      </div>
    </>
  );
}
