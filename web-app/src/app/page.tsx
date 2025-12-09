"use client";

import { useState, useEffect, Suspense } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { Session } from "@/gen/session/v1/types_pb";
import { SessionList } from "@/components/sessions/SessionList";
import { SessionListSkeleton } from "@/components/sessions/SessionListSkeleton";
import { SessionDetail } from "@/components/sessions/SessionDetail";
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
  const [showHelp, setShowHelp] = useState(false);
  const [isSessionFullscreen, setIsSessionFullscreen] = useState(false);
  const [pendingSessionId, setPendingSessionId] = useState<string | null>(null);

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
    listSessions,
  } = useSessionService({
    baseUrl: getApiBaseUrl(),
    autoWatch: true,
    onNotification: handleNotification,
  });

  // Handle pending session navigation from notification click
  useEffect(() => {
    if (pendingSessionId && sessions.length > 0) {
      const session = sessions.find((s) => s.id === pendingSessionId);
      if (session) {
        setSelectedSession(session);
      }
      setPendingSessionId(null);
    }
  }, [pendingSessionId, sessions]);

  // Handle direct session selection from URL (e.g., from review queue)
  useEffect(() => {
    const sessionId = searchParams.get("session");
    if (sessionId) {
      const session = sessions.find((s) => s.id === sessionId);
      if (session) {
        setSelectedSession(session);
      }
    }
  }, [searchParams, sessions]);

  // Handle session deletion - close modal first if this session is selected
  const handleDeleteSession = async (sessionId: string) => {
    // Close modal if we're deleting the currently selected session
    // This ensures WebSocket cleanup happens before deletion
    if (selectedSession?.id === sessionId) {
      setSelectedSession(null);
      // Small delay to let cleanup complete
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    await deleteSession(sessionId);
  };

  // Handle session duplication
  const handleDuplicateSession = (sessionId: string) => {
    router.push(`/sessions/new?duplicate=${sessionId}`);
  };

  // Keyboard shortcuts
  useKeyboard({
    "?": () => setShowHelp(true),
    Escape: () => {
      if (showHelp) {
        setShowHelp(false);
      } else if (selectedSession) {
        setSelectedSession(null);
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
            onSessionClick={(session) => setSelectedSession(session)}
            onDeleteSession={handleDeleteSession}
            onPauseSession={pauseSession}
            onResumeSession={resumeSession}
            onDuplicateSession={handleDuplicateSession}
          />
        )}
      </main>

      {/* Session detail modal */}
      {selectedSession && (
        <div className={styles.modal} onClick={() => setSelectedSession(null)}>
          <div
            className={`${styles.modalContent} ${isSessionFullscreen ? styles.modalContentFullscreen : ""}`}
            onClick={(e) => e.stopPropagation()}
          >
            <SessionDetail
              session={selectedSession}
              onClose={() => setSelectedSession(null)}
              onFullscreenChange={setIsSessionFullscreen}
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
