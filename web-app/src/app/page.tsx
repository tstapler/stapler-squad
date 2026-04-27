"use client";
// +feature: session-list session-search session-filter session-groupby

import { useState, useEffect, useRef, Suspense, useCallback } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { Session } from "@/gen/session/v1/types_pb";
import { SessionList } from "@/components/sessions/SessionList";
import { SessionListSkeleton } from "@/components/sessions/SessionListSkeleton";
import { SessionDetail, SessionDetailTab } from "@/components/sessions/SessionDetail";
import { SessionWizard } from "@/components/sessions/SessionWizard";
import { ResumeSessionModal } from "@/components/sessions/ResumeSessionModal";
import { ErrorState } from "@/components/ui/ErrorState";
import { KeyboardHints } from "@/components/ui/KeyboardHint";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { useSessionNotifications } from "@/lib/hooks/useSessionNotifications";
import { useKeyboard } from "@/lib/hooks/useKeyboard";
import { useAuth } from "@/lib/contexts/AuthContext";
import { useOmnibar } from "@/lib/contexts/OmnibarContext";
import { getApiBaseUrl } from "@/lib/config";
import { SessionFormData } from "@/lib/validation/sessionSchema";
import * as styles from "./page.css";

function HomeContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const { authEnabled, authenticated, loading: authLoading } = useAuth();
  const { openInCreationMode } = useOmnibar();
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);
  const [activeTab, setActiveTab] = useState<SessionDetailTab>("info");
  const [isHelpOpen, setShowHelp] = useState(false);
  const [isSessionFullscreen, setIsSessionFullscreen] = useState(false);
  const [pendingSessionId, setPendingSessionId] = useState<string | null>(null);

  // Keep the last visible session alive so SessionDetail doesn't unmount on modal close
  const lastVisibleSessionRef = useRef<Session | null>(null);
  if (selectedSession) {
    lastVisibleSessionRef.current = selectedSession;
  }
  const modalSession = lastVisibleSessionRef.current;

  // Resume modal state
  const [resumeTarget, setResumeTarget] = useState<Session | null>(null);

  // Wizard modal state
  const [showWizard, setShowWizard] = useState(false);
  const [wizardInitialData, setWizardInitialData] = useState<Partial<SessionFormData> | undefined>(undefined);
  // Track whether wizard was opened via query params so we clean up URL on close
  const openedViaQueryParam = useRef(false);

  // Focus management: modal containers (tabIndex={-1}) and trigger element refs
  const sessionModalContentRef = useRef<HTMLDivElement>(null);
  const wizardModalContentRef = useRef<HTMLDivElement>(null);
  const helpModalContentRef = useRef<HTMLDivElement>(null);
  const sessionTriggerRef = useRef<HTMLElement | null>(null);
  const wizardTriggerRef = useRef<HTMLElement | null>(null);
  const helpTriggerRef = useRef<HTMLElement | null>(null);

  // Focus session modal when it opens; return focus on close
  useEffect(() => {
    if (selectedSession) {
      sessionModalContentRef.current?.focus();
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

  // Focus help modal when it opens; return focus on close
  useEffect(() => {
    if (isHelpOpen) {
      helpModalContentRef.current?.focus();
    } else if (helpTriggerRef.current) {
      helpTriggerRef.current.focus();
      helpTriggerRef.current = null;
    }
  }, [isHelpOpen]);

  // Valid tab values for URL parsing
  const validTabs: SessionDetailTab[] = ["terminal", "diff", "vcs", "logs", "info"];
  const isValidTab = (tab: string | null): tab is SessionDetailTab =>
    tab !== null && validTabs.includes(tab as SessionDetailTab);

  // Notification handler for session events
  const handleNotification = useSessionNotifications({
    enableAudio: true,
    onViewSession: (sessionId) => {
      // Store the session ID to navigate to; we'll resolve it when sessions are available
      setPendingSessionId(sessionId);
    },
  });

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
    createCheckpoint,
    listCheckpoints,
    forkSession,
    runOneShot,
    listSessions,
    updateSession,
    getSession,
  } = useSessionService({
    baseUrl: getApiBaseUrl(),
    autoWatch: true,
    enabled: !authLoading && (!authEnabled || authenticated),
    onNotification: handleNotification,
  });

  // Helper function to find a session by ID with fuzzy matching for external sessions
  // This handles multiple matching scenarios:
  // 1. Exact ID match (session title)
  // 2. ID prefix match (for suffixed session IDs like "session (External)")
  // 3. External metadata tmux session name match
  // 4. Tmux session name with prefix stripped (e.g., "claudesquad_foo" → "foo")
  // 5. Path-based matching (for notifications from hooks using cwd)
  const findSessionById = useCallback((sessionId: string): Session | undefined => {
    // Try exact ID match first
    let session = sessions.find((s) => s.id === sessionId);
    if (session) return session;

    // If no exact match, try fuzzy matching for external sessions
    session = sessions.find((s) => {
      // Check if the session ID starts with the search ID
      if (s.id.startsWith(sessionId)) {
        return true;
      }
      // Check external metadata for tmux session name match
      if (s.externalMetadata?.tmuxSessionName === sessionId) {
        return true;
      }
      // Check if the search ID is contained in the session path (for cwd-based lookups)
      // This handles cases where hooks send the full directory path
      if (sessionId.includes("/") && s.path && s.path.includes(sessionId)) {
        return true;
      }
      // Check if the session path ends with the search ID (directory name matching)
      if (s.path && s.path.endsWith(`/${sessionId}`)) {
        return true;
      }
      return false;
    });

    // If still no match, try stripping tmux prefix and matching
    // Handle cases where notification sends "claudesquad_foo" but session.id is "foo"
    if (!session && sessionId.includes("_")) {
      const withoutPrefix = sessionId.split("_").slice(1).join("_"); // Strip first part before underscore
      session = sessions.find((s) => s.id === withoutPrefix || s.title === withoutPrefix);
    }

    // If still no match, try matching by title or path basename
    if (!session) {
      const searchLower = sessionId.toLowerCase();
      session = sessions.find((s) => {
        // Title match (case-insensitive)
        if (s.title.toLowerCase() === searchLower) {
          return true;
        }
        // Path basename match
        const pathBasename = s.path?.split("/").pop()?.toLowerCase();
        if (pathBasename === searchLower) {
          return true;
        }
        return false;
      });
    }

    // Log if no session found for debugging
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
        // Navigate to terminal tab for notifications (user likely needs to see/interact with terminal)
        setActiveTab("terminal");
        updateUrl(session.id, "terminal");
      } else {
        console.warn(`[Notification] Session not found: ${pendingSessionId}`);
      }
      setPendingSessionId(null);
    }
  }, [pendingSessionId, sessions]);

  // Handle direct session selection from URL (e.g., from review queue, deep links)
  useEffect(() => {
    const sessionId = searchParams.get("session");
    const tabParam = searchParams.get("tab");
    if (sessionId && sessions.length > 0) {
      const session = findSessionById(sessionId);
      if (session) {
        setSelectedSession(session);
        // Set tab from URL or default to "terminal" for notification deep links
        if (isValidTab(tabParam)) {
          setActiveTab(tabParam);
        } else {
          // Default to terminal tab for deep links (notifications)
          setActiveTab("terminal");
        }
      } else {
        console.warn(`[URL] Session not found: ${sessionId}`);
      }
    }
  }, [searchParams, sessions]);

  // Detect ?new=true or ?duplicate=<id> query params and auto-open wizard
  useEffect(() => {
    const newParam = searchParams.get("new");
    const duplicateId = searchParams.get("duplicate");

    if (newParam === "true") {
      setWizardInitialData(undefined);
      setShowWizard(true);
      openedViaQueryParam.current = true;
      // Clean the URL immediately so refresh doesn't re-open the wizard
      router.replace("/", { scroll: false });
    } else if (duplicateId) {
      openedViaQueryParam.current = true;
      // Clean the URL immediately before async session load
      router.replace("/", { scroll: false });
      // Load session data for duplication
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
        // If loading the session fails, still open wizard without initial data
        setShowWizard(true);
      });
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

  // Handle session deletion - close modal first if this session is selected
  const handleDeleteSession = async (sessionId: string) => {
    // Close modal if we're deleting the currently selected session
    // This ensures WebSocket cleanup happens before deletion
    if (selectedSession?.id === sessionId) {
      closeSession();
      // Small delay to let cleanup complete
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    await deleteSession(sessionId);
  };

  // Handle new workspace on same project - open wizard with path/program/category pre-filled but fresh title
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

  // Handle session clone - open omnibar in creation mode
  const handleCloneSession = useCallback((_sessionId: string) => {
    openInCreationMode();
  }, [openInCreationMode]);

  // Handle new session - open wizard modal
  const handleNewSession = () => {
    wizardTriggerRef.current = document.activeElement as HTMLElement;
    openedViaQueryParam.current = false;
    setWizardInitialData(undefined);
    setShowWizard(true);
  };

  // Handle wizard completion
  const handleWizardComplete = async (data: SessionFormData) => {
    // If useTitleAsBranch is checked, use the session title as the branch name
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

  // Handle wizard cancel
  const handleWizardCancel = () => {
    setShowWizard(false);
    setWizardInitialData(undefined);
    if (openedViaQueryParam.current) {
      router.replace("/", { scroll: false });
      openedViaQueryParam.current = false;
    }
  };

  // Handle tag updates - sends non-empty tag arrays; clearing all tags is not yet supported
  // (proto3 repeated fields cannot distinguish "not set" from "empty array")
  const handleUpdateTags = async (sessionId: string, tags: string[]) => {
    if (tags.length > 0) {
      await updateSession(sessionId, { tags });
    }
  };

  // Handle one-shot PR creation (S3-3)
  const handleRunOneShot = useCallback(async (sessionId: string): Promise<void> => {
    await runOneShot(sessionId, "Create a pull request for the changes in this session.", 0);
  }, [runOneShot]);

  // Handle resume request - show modal for user to edit title/tags before resuming
  const handleResumeRequest = useCallback((session: Session) => {
    setResumeTarget(session);
  }, []);

  // Handle direct resume (bulk mode) - resume immediately without showing the modal
  const handleDirectResume = useCallback((session: Session) => {
    resumeSession(session.id, { title: session.title, tags: [...(session.tags || [])] });
  }, [resumeSession]);

  // Handle resume confirm - apply updates and resume session
  // Only close the modal on success; keep it open on error so the user can retry
  const handleResumeConfirm = useCallback(async (updates: { title: string; tags: string[] }) => {
    if (!resumeTarget) return;
    try {
      await resumeSession(resumeTarget.id, updates);
      setResumeTarget(null);
    } catch {
      // resumeSession dispatches to Redux error state; modal stays open for retry
    }
  }, [resumeTarget, resumeSession]);

  // Handle resume cancel
  const handleResumeCancel = useCallback(() => {
    setResumeTarget(null);
  }, []);

  // Handle session selection with URL update
  const handleSessionClick = (session: Session) => {
    sessionTriggerRef.current = document.activeElement as HTMLElement;
    if (typeof performance !== "undefined") {
      performance.mark("session:click");
    }
    setSelectedSession(session);
    setActiveTab("info");
    updateUrl(session.id, "info");
  };

  // Handle tab changes with URL update
  const handleTabChange = (tab: SessionDetailTab) => {
    setActiveTab(tab);
    if (selectedSession) {
      updateUrl(selectedSession.id, tab);
    }
  };

  // Keyboard shortcuts
  useKeyboard({
    "?": () => { helpTriggerRef.current = document.activeElement as HTMLElement; setShowHelp(true); },
    Escape: () => {
      if (resumeTarget) {
        setResumeTarget(null);
      } else if (showWizard) {
        handleWizardCancel();
      } else if (isHelpOpen) {
        setShowHelp(false);
      } else if (selectedSession) {
        closeSession();
      }
    },
    "R": () => !loading && listSessions(),
  });

  return (
    <div className={styles.page}>
      <main id="main-content" className={styles.main}>
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
          />
        )}
      </main>

      {/* Session detail modal - kept alive across close/reopen to preserve xterm.js terminals */}
      <div
        className={styles.modal}
        aria-hidden={!selectedSession}
        style={{ display: selectedSession ? undefined : 'none' }}
        onClick={closeSession}
      >
        <div
          ref={sessionModalContentRef}
          tabIndex={-1}
          role="dialog"
          aria-modal="true"
          aria-label="Session detail"
          className={`${styles.modalContent} ${isSessionFullscreen ? styles.modalContentFullscreen : ""}`}
          onClick={(e) => e.stopPropagation()}
        >
          {modalSession && (
            <SessionDetail
              session={modalSession}
              onClose={closeSession}
              onFullscreenChange={setIsSessionFullscreen}
              onTabChange={handleTabChange}
              initialTab={activeTab}
            />
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

      {/* Keyboard shortcuts help modal */}
      {isHelpOpen && (
        <div className={styles.modal} onClick={() => setShowHelp(false)}>
          <div ref={helpModalContentRef} tabIndex={-1} className={styles.modalContent} onClick={(e) => e.stopPropagation()}>
            <div className={styles.modalHeader}>
              <h2>Keyboard Shortcuts</h2>
              <button
                className={styles.closeButton}
                onClick={() => setShowHelp(false)}
                aria-label="Close"
              >
                ✕
              </button>
            </div>
            <div className={styles.modalBody}>
              <KeyboardHints
                hints={[
                  { keys: "?", description: "Show keyboard shortcuts" },
                  { keys: "Escape", description: "Close modal / dialog" },
                  { keys: "R", description: "Refresh session list" },
                  { keys: "Enter", description: "Open selected session" },
                ]}
              />
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
