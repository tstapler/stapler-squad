"use client";

import { useState, useEffect, Suspense } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { Session } from "@/gen/session/v1/types_pb";
import { SessionList } from "@/components/sessions/SessionList";
import { SessionListSkeleton } from "@/components/sessions/SessionListSkeleton";
import { SessionDetail, SessionDetailTab } from "@/components/sessions/SessionDetail";
import { ErrorState } from "@/components/ui/ErrorState";
import { KeyboardHints } from "@/components/ui/KeyboardHint";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { useSessionNotifications } from "@/lib/hooks/useSessionNotifications";
import { useKeyboard } from "@/lib/hooks/useKeyboard";
import { getApiBaseUrl } from "@/lib/config";
import styles from "./page.module.css";

function HomeContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);
  const [activeTab, setActiveTab] = useState<SessionDetailTab>("info");
  const [showHelp, setShowHelp] = useState(false);
  const [isSessionFullscreen, setIsSessionFullscreen] = useState(false);
  const [pendingSessionId, setPendingSessionId] = useState<string | null>(null);

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
    deleteSession,
    pauseSession,
    resumeSession,
    renameSession,
    restartSession,
    listSessions,
    updateSession,
  } = useSessionService({
    baseUrl: getApiBaseUrl(),
    autoWatch: true,
    onNotification: handleNotification,
  });

  // Helper function to find a session by ID with fuzzy matching for external sessions
  // This handles multiple matching scenarios:
  // 1. Exact ID match (session title)
  // 2. ID prefix match (for suffixed session IDs like "session (External)")
  // 3. External metadata tmux session name match
  // 4. Path-based matching (for notifications from hooks using cwd)
  const findSessionById = (sessionId: string): Session | undefined => {
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
  };

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
        // Set tab from URL or default to "info"
        if (isValidTab(tabParam)) {
          setActiveTab(tabParam);
        }
      } else {
        console.warn(`[URL] Session not found: ${sessionId}`);
      }
    }
  }, [searchParams, sessions]);

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

  // Handle session duplication
  const handleDuplicateSession = (sessionId: string) => {
    router.push(`/sessions/new?duplicate=${sessionId}`);
  };

  // Handle tag updates - TODO: requires adding 'tags' field to UpdateSessionRequest proto
  const handleUpdateTags = async (sessionId: string, tags: string[]) => {
    // Tags not yet supported in UpdateSessionRequest proto
    // await updateSession(sessionId, { tags });
    console.warn('Tag updates not yet implemented in proto');
  };

  // Handle session selection with URL update
  const handleSessionClick = (session: Session) => {
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
    "?": () => setShowHelp(true),
    Escape: () => {
      if (showHelp) {
        setShowHelp(false);
      } else if (selectedSession) {
        closeSession();
      }
    },
    "R": () => !loading && listSessions(),
  });

  return (
    <div className={styles.page}>
      <main className={styles.main}>
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
            onResumeSession={resumeSession}
            onDuplicateSession={handleDuplicateSession}
            onRenameSession={renameSession}
            onRestartSession={restartSession}
            onUpdateTags={handleUpdateTags}
          />
        )}
      </main>

      {/* Session detail modal */}
      {selectedSession && (
        <div className={styles.modal} onClick={closeSession}>
          <div
            className={`${styles.modalContent} ${isSessionFullscreen ? styles.modalContentFullscreen : ""}`}
            onClick={(e) => e.stopPropagation()}
          >
            <SessionDetail
              session={selectedSession}
              onClose={closeSession}
              onFullscreenChange={setIsSessionFullscreen}
              onTabChange={handleTabChange}
              initialTab={activeTab}
            />
          </div>
        </div>
      )}

      {/* Keyboard shortcuts help modal */}
      {showHelp && (
        <div className={styles.modal} onClick={() => setShowHelp(false)}>
          <div className={styles.modalContent} onClick={(e) => e.stopPropagation()}>
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
                  { keys: "/", description: "Focus search (coming soon)" },
                  { keys: ["↑", "↓"], description: "Navigate sessions (coming soon)" },
                ]}
              />
            </div>
          </div>
        </div>
      )}

      {/* Floating help button */}
      <button
        className={styles.helpButton}
        onClick={() => setShowHelp(true)}
        aria-label="Show keyboard shortcuts"
        title="Keyboard shortcuts (?)"
      >
        ?
      </button>
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
