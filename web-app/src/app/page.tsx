"use client";
// +feature: session-list session-search session-filter session-groupby

import { useState, useEffect, useRef, Suspense, useCallback } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { Session } from "@/gen/session/v1/types_pb";
import { SessionList } from "@/components/sessions/SessionList";
import { SessionListSkeleton } from "@/components/sessions/SessionListSkeleton";
import { SessionDetail, SessionDetailTab } from "@/components/sessions/SessionDetail";
import { SessionDetailBar } from "@/components/sessions/SessionDetailBar";
import { SessionWizard } from "@/components/sessions/SessionWizard";
import { ResumeSessionModal } from "@/components/sessions/ResumeSessionModal";
import { ErrorState } from "@/components/ui/ErrorState";
import { useSessionServiceContext } from "@/lib/contexts/SessionServiceContext";
import { useKeyboard } from "@/lib/hooks/useKeyboard";
import { useOmnibar } from "@/lib/contexts/OmnibarContext";
import { SessionFormData } from "@/lib/validation/sessionSchema";
import {
  cockpitGrid,
  sessionListColumn,
  detailColumn,
} from "@/styles/sessionCockpit.css";
import * as styles from "./page.css";

function HomeContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const { openInCreationMode } = useOmnibar();
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);
  const [activeTab, setActiveTab] = useState<SessionDetailTab>("info");
  const [deleteConfirmTarget, setDeleteConfirmTarget] = useState<Session | null>(null);
  const [isSessionFullscreen, setIsSessionFullscreen] = useState(false);
  const [pendingSessionId, setPendingSessionId] = useState<string | null>(null);
  // j/k keyboard navigation index within the session list
  const [focusedSessionIndex, setFocusedSessionIndex] = useState<number>(-1);

  // Keep the last visible session alive so SessionDetail doesn't unmount on deselect
  const lastVisibleSessionRef = useRef<Session | null>(null);
  if (selectedSession) {
    lastVisibleSessionRef.current = selectedSession;
  }
  const detailSession = lastVisibleSessionRef.current;

  // Resume modal state
  const [resumeTarget, setResumeTarget] = useState<Session | null>(null);

  // Wizard modal state
  const [showWizard, setShowWizard] = useState(false);
  const [wizardInitialData, setWizardInitialData] = useState<Partial<SessionFormData> | undefined>(undefined);
  // Track whether wizard was opened via query params so we clean up URL on close
  const openedViaQueryParam = useRef(false);

  // Focus management: modal containers (tabIndex={-1}) and trigger element refs
  const sessionDetailRef = useRef<HTMLDivElement>(null);
  const wizardModalContentRef = useRef<HTMLDivElement>(null);
  const sessionTriggerRef = useRef<HTMLElement | null>(null);
  const wizardTriggerRef = useRef<HTMLElement | null>(null);

  // Focus detail panel when session opens; return focus on close
  useEffect(() => {
    if (selectedSession) {
      sessionDetailRef.current?.focus();
    } else if (sessionTriggerRef.current) {
      sessionTriggerRef.current.focus();
      sessionTriggerRef.current = null;
    }
  }, [selectedSession]);

  // Focus wizard modal when it opens; return focus on close
  useEffect(() => {
    if (showWizard) {
      wizardModalContentRef.current?.focus();
    } else if (wizardTriggerRef.current) {
      wizardTriggerRef.current.focus();
      wizardTriggerRef.current = null;
    }
  }, [showWizard]);

  // Valid tab values for URL parsing
  const validTabs: SessionDetailTab[] = ["terminal", "diff", "vcs", "logs", "info"];
  const isValidTab = (tab: string | null): tab is SessionDetailTab =>
    tab !== null && validTabs.includes(tab as SessionDetailTab);

  const {
    sessions,
    loading,
    error,
    createSession,
    deleteSession,
    pauseSession,
    resumeSession,
    renameSession,
    restartSession,
    clearConversationState,
    createCheckpoint,
    listCheckpoints,
    forkSession,
    runOneShot,
    listSessions,
    updateSession,
    getSession,
  } = useSessionServiceContext();

  // Helper function to find a session by ID with fuzzy matching for external sessions
  const findSessionById = useCallback((sessionId: string): Session | undefined => {
    let session = sessions.find((s) => s.id === sessionId);
    if (session) return session;

    session = sessions.find((s) => {
      if (s.id.startsWith(sessionId)) return true;
      if (s.externalMetadata?.tmuxSessionName === sessionId) return true;
      if (sessionId.includes("/") && s.path && s.path.includes(sessionId)) return true;
      if (s.path && s.path.endsWith(`/${sessionId}`)) return true;
      return false;
    });

    if (!session && sessionId.includes("_")) {
      const withoutPrefix = sessionId.split("_").slice(1).join("_");
      session = sessions.find((s) => s.id === withoutPrefix || s.title === withoutPrefix);
    }

    if (!session) {
      const searchLower = sessionId.toLowerCase();
      session = sessions.find((s) => {
        if (s.title.toLowerCase() === searchLower) return true;
        const pathBasename = s.path?.split("/").pop()?.toLowerCase();
        if (pathBasename === searchLower) return true;
        return false;
      });
    }

    if (!session) {
      console.warn(`[findSessionById] No session found for ID: ${sessionId}`, {
        availableSessions: sessions.map(s => ({ id: s.id, title: s.title, path: s.path }))
      });
    }

    return session;
  }, [sessions]);

  // Handle pending session navigation from notification click
  useEffect(() => {
    if (pendingSessionId && sessions.length > 0) {
      const session = findSessionById(pendingSessionId);
      if (session) {
        setSelectedSession(session);
        setActiveTab("terminal");
        updateUrl(session.id, "terminal");
      } else {
        console.warn(`[Notification] Session not found: ${pendingSessionId}`);
      }
      setPendingSessionId(null);
    }
  }, [pendingSessionId, sessions]);

  // Handle direct session selection from URL
  useEffect(() => {
    const sessionId = searchParams.get("session");
    const tabParam = searchParams.get("tab");
    if (sessionId && sessions.length > 0) {
      const session = findSessionById(sessionId);
      if (session) {
        setSelectedSession(session);
        if (isValidTab(tabParam)) {
          setActiveTab(tabParam);
        } else {
          setActiveTab("terminal");
        }
      } else {
        console.warn(`[URL] Session not found: ${sessionId}`);
      }
    }
  }, [searchParams, sessions]);

  // Detect ?new=true, ?duplicate=<id>, or ?worktree=<path>&branch=<branch> query params
  useEffect(() => {
    const newParam = searchParams.get("new");
    const duplicateId = searchParams.get("duplicate");
    const worktreePath = searchParams.get("worktree");
    const worktreeBranch = searchParams.get("branch");
    const worktreeTitle = searchParams.get("title");

    if (newParam === "true") {
      setWizardInitialData(undefined);
      setShowWizard(true);
      openedViaQueryParam.current = true;
      router.replace("/", { scroll: false });
    } else if (duplicateId) {
      openedViaQueryParam.current = true;
      router.replace("/", { scroll: false });
      getSession(duplicateId).then((session) => {
        if (session) {
          setWizardInitialData({
            title: `${session.title}-copy`,
            path: session.path,
            workingDir: session.workingDir || "",
            branch: session.branch || "",
            program: session.program || "claude",
            category: session.category || "",
            prompt: "",
            autoYes: false,
          });
        }
        setShowWizard(true);
      }).catch(() => {
        setShowWizard(true);
      });
    } else if (worktreePath) {
      openedViaQueryParam.current = true;
      router.replace("/", { scroll: false });
      setWizardInitialData({
        title: worktreeTitle || "",
        path: worktreePath,
        branch: worktreeBranch || "",
        workingDir: "",
        program: "claude",
        category: "",
        prompt: "",
        autoYes: false,
      });
      setShowWizard(true);
    }
  }, [searchParams, getSession]);

  // Update URL with session and tab parameters
  const updateUrl = (sessionId: string | null, tab: SessionDetailTab | null) => {
    const params = new URLSearchParams();
    if (sessionId) {
      params.set("session", sessionId);
      if (tab && tab !== "info") {
        params.set("tab", tab);
      }
    }
    const query = params.toString();
    router.replace(query ? `/?${query}` : "/", { scroll: false });
  };

  // Close session and clear URL query parameter
  const closeSession = () => {
    setSelectedSession(null);
    setActiveTab("info");
    updateUrl(null, null);
  };

  // Handle session deletion
  const handleDeleteSession = async (sessionId: string) => {
    if (selectedSession?.id === sessionId) {
      closeSession();
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    await deleteSession(sessionId);
  };

  // Handle new workspace on same project
  const handleNewWorkspaceSession = (sessionId: string) => {
    wizardTriggerRef.current = document.activeElement as HTMLElement;
    openedViaQueryParam.current = false;
    getSession(sessionId).then((session) => {
      if (session) {
        setWizardInitialData({
          path: session.path,
          workingDir: session.workingDir || "",
          program: session.program || "claude",
          category: session.category || "",
          title: "",
          branch: "",
          prompt: "",
          autoYes: false,
          useTitleAsBranch: true,
        });
      }
      setShowWizard(true);
    }).catch(() => {
      setShowWizard(true);
    });
  };

  const handleCloneSession = useCallback((_sessionId: string) => {
    openInCreationMode();
  }, [openInCreationMode]);

  const handleNewSession = () => {
    wizardTriggerRef.current = document.activeElement as HTMLElement;
    openedViaQueryParam.current = false;
    setWizardInitialData(undefined);
    setShowWizard(true);
  };

  const handleWizardComplete = async (data: SessionFormData) => {
    const branchName = data.useTitleAsBranch ? data.title : (data.branch || "");
    await createSession({
      title: data.title,
      path: data.path,
      workingDir: data.workingDir || "",
      branch: branchName,
      program: data.program,
      category: data.category || "",
      prompt: data.prompt || "",
      initialPrompt: data.initialPrompt || "",
      autoYes: data.autoYes,
      existingWorktree: data.existingWorktree || "",
    });
    setShowWizard(false);
    setWizardInitialData(undefined);
    if (openedViaQueryParam.current) {
      router.replace("/", { scroll: false });
      openedViaQueryParam.current = false;
    }
  };

  const handleWizardCancel = () => {
    setShowWizard(false);
    setWizardInitialData(undefined);
    if (openedViaQueryParam.current) {
      router.replace("/", { scroll: false });
      openedViaQueryParam.current = false;
    }
  };

  const handleUpdateTags = async (sessionId: string, tags: string[]) => {
    if (tags.length > 0) {
      await updateSession(sessionId, { tags });
    }
  };

  const handleSetRateLimitEnabled = useCallback(async (sessionId: string, enabled: boolean): Promise<void> => {
    await updateSession(sessionId, { rateLimitEnabled: enabled });
  }, [updateSession]);

  const handleRunOneShot = useCallback(async (sessionId: string): Promise<void> => {
    await runOneShot(sessionId, "Create a pull request for the changes in this session.", 0);
  }, [runOneShot]);

  const handleResumeRequest = useCallback((session: Session) => {
    setResumeTarget(session);
  }, []);

  const handleDirectResume = useCallback((session: Session) => {
    resumeSession(session.id, { title: session.title, tags: [...(session.tags || [])] });
  }, [resumeSession]);

  const handleResumeConfirm = useCallback(async (updates: { title: string; tags: string[] }) => {
    if (!resumeTarget) return;
    try {
      await resumeSession(resumeTarget.id, updates);
      setResumeTarget(null);
    } catch {
      // resumeSession dispatches to Redux error state; modal stays open for retry
    }
  }, [resumeTarget, resumeSession]);

  const handleResumeCancel = useCallback(() => {
    setResumeTarget(null);
  }, []);

  const handleSessionClick = (session: Session) => {
    sessionTriggerRef.current = document.activeElement as HTMLElement;
    if (typeof performance !== "undefined") {
      performance.mark("session:click");
    }
    setSelectedSession(session);
    setActiveTab("info");
    updateUrl(session.id, "info");
  };

  const handleTabChange = (tab: SessionDetailTab) => {
    setActiveTab(tab);
    if (selectedSession) {
      updateUrl(selectedSession.id, tab);
    }
  };

  // Story 3.2 — j/k keyboard navigation in session list
  // When no session is open, j/k move the focus index; Enter opens the focused session.
  // When a session is open, p/r/d act on the currently-open session.
  useKeyboard({
    // '?' is handled exclusively by CockpitShell's useShortcut to avoid dual-listener collision
    Escape: () => {
      if (deleteConfirmTarget) {
        setDeleteConfirmTarget(null);
      } else if (resumeTarget) {
        setResumeTarget(null);
      } else if (showWizard) {
        handleWizardCancel();
      } else if (selectedSession) {
        closeSession();
      }
    },
    "R": () => !loading && listSessions(),
    // j/k navigation (only when no modal is open)
    "j": () => {
      if (showWizard || deleteConfirmTarget || resumeTarget) return;
      setFocusedSessionIndex(prev =>
        sessions.length === 0 ? -1 : Math.min(prev + 1, sessions.length - 1)
      );
    },
    "k": () => {
      if (showWizard || deleteConfirmTarget || resumeTarget) return;
      setFocusedSessionIndex(prev =>
        sessions.length === 0 ? -1 : Math.max(prev - 1, 0)
      );
    },
    Enter: () => {
      if (showWizard || deleteConfirmTarget || resumeTarget) return;
      if (!selectedSession && focusedSessionIndex >= 0 && sessions[focusedSessionIndex]) {
        handleSessionClick(sessions[focusedSessionIndex]);
      }
    },
    // p/r/d act on the open session
    "p": () => {
      if (selectedSession && !showWizard && !deleteConfirmTarget) {
        pauseSession(selectedSession.id);
      }
    },
    "r": () => {
      if (selectedSession && !showWizard && !deleteConfirmTarget) {
        handleResumeRequest(selectedSession);
      }
    },
    "d": () => {
      if (selectedSession && !showWizard && !deleteConfirmTarget) {
        setDeleteConfirmTarget(selectedSession);
      }
    },
    // t — jump to terminal tab
    "t": () => {
      if (selectedSession && !showWizard && !deleteConfirmTarget) {
        handleTabChange("terminal");
      }
    },
  });

  return (
    <div className={styles.page}>
      {/* 3-column cockpit grid: session list | detail panel | (context panel future) */}
      <div
        className={cockpitGrid({ contextPanelOpen: false })}
        style={{ flex: 1, minHeight: 0 }}
      >
        {/* Column 1 — session list */}
        <div className={sessionListColumn}>
          {loading && <SessionListSkeleton count={4} />}
          {error && !loading && (
            <ErrorState
              error={error}
              title="Failed to Load Sessions"
              message="Unable to connect to the server. Please check that the server is running and try again."
              onRetry={() => listSessions()}
            />
          )}
          {!loading && !error && (
            <SessionList
              sessions={sessions}
              onSessionClick={handleSessionClick}
              onDeleteSession={handleDeleteSession}
              onPauseSession={pauseSession}
              onResumeSession={handleResumeRequest}
              onDirectResumeSession={handleDirectResume}
              onCloneSession={handleCloneSession}
              onNewWorkspaceSession={handleNewWorkspaceSession}
              onRenameSession={renameSession}
              onRestartSession={restartSession}
              onUpdateTags={handleUpdateTags}
              onNewSession={handleNewSession}
              onCreateCheckpoint={createCheckpoint}
              onListCheckpoints={listCheckpoints}
              onForkFromCheckpoint={forkSession}
              onRunOneShot={handleRunOneShot}
              onSetRateLimitEnabled={handleSetRateLimitEnabled}
              onClearConversationState={clearConversationState}
            />
          )}
        </div>

        {/* Column 2 — session detail / terminal */}
        <div className={detailColumn}>
          {detailSession ? (
            <>
              {/* Story 2.2.4 — compact session detail bar above terminal */}
              <SessionDetailBar
                branch={detailSession.branch}
                path={detailSession.path}
                onBack={closeSession}
              />
              <div
                ref={sessionDetailRef}
                tabIndex={-1}
                role="region"
                aria-label="Session detail"
                style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}
              >
                <SessionDetail
                  session={detailSession}
                  onClose={closeSession}
                  onFullscreenChange={setIsSessionFullscreen}
                  onTabChange={handleTabChange}
                  initialTab={activeTab}
                />
              </div>
            </>
          ) : (
            <div className={styles.placeholder}>
              Select a session to view details
            </div>
          )}
        </div>
      </div>

      {/* Session creation wizard modal */}
      {showWizard && (
        <div className={styles.modal} onClick={handleWizardCancel}>
          <div ref={wizardModalContentRef} tabIndex={-1} className={styles.modalContent} onClick={(e) => e.stopPropagation()}>
            <div className={styles.modalHeader}>
              <h2>{wizardInitialData ? "Duplicate Session" : "Create New Session"}</h2>
              <button
                className={styles.closeButton}
                onClick={handleWizardCancel}
                aria-label="Close"
              >
                ✕
              </button>
            </div>
            <div className={styles.modalBody}>
              <SessionWizard
                onComplete={handleWizardComplete}
                onCancel={handleWizardCancel}
                initialData={wizardInitialData}
                existingTitles={sessions.map((s) => s.title)}
              />
            </div>
          </div>
        </div>
      )}

      {/* Resume session modal */}
      {resumeTarget && (
        <ResumeSessionModal
          key={resumeTarget.id}
          session={resumeTarget}
          sessions={sessions}
          onConfirm={handleResumeConfirm}
          onCancel={handleResumeCancel}
        />
      )}

      {/* Delete confirmation modal (triggered by 'd' keyboard shortcut) */}
      {deleteConfirmTarget && (
        <div className={styles.modal} onClick={() => setDeleteConfirmTarget(null)}>
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="deleteConfirmTitle"
            tabIndex={-1}
            className={styles.modalContent}
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => { if (e.key === "Escape") setDeleteConfirmTarget(null); }}
          >
            <div className={styles.modalHeader}>
              <h2 id="deleteConfirmTitle">Delete Session</h2>
              <button className={styles.closeButton} onClick={() => setDeleteConfirmTarget(null)} aria-label="Close">✕</button>
            </div>
            <div className={styles.modalBody}>
              <p>Delete &quot;{deleteConfirmTarget.title}&quot;?</p>
              <p style={{ color: "var(--error, #ef4444)", fontSize: "0.875rem", marginTop: "0.5rem" }}>This action cannot be undone.</p>
              <div style={{ display: "flex", gap: "0.5rem", justifyContent: "flex-end", marginTop: "1.5rem" }}>
                <button className={styles.cancelButton} onClick={() => setDeleteConfirmTarget(null)}>Cancel</button>
                <button
                  className={styles.dangerButton}
                  onClick={async () => {
                    const target = deleteConfirmTarget;
                    setDeleteConfirmTarget(null);
                    await handleDeleteSession(target.id);
                  }}
                >
                  Delete
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export default function Home() {
  return (
    <Suspense fallback={<SessionListSkeleton count={4} />}>
      <HomeContent />
    </Suspense>
  );
}
