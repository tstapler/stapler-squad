"use client";

import { useState, useEffect, Suspense } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { Session } from "@/gen/session/v1/types_pb";
import { ReviewQueuePanel } from "@/components/sessions/ReviewQueuePanel";
import { SessionDetail, SessionDetailTab } from "@/components/sessions/SessionDetail";
import { useSessionService } from "@/lib/hooks/useSessionService";
import styles from "./page.module.css";

function ReviewQueueContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);
  const [selectedTab, setSelectedTab] = useState<SessionDetailTab>("terminal");
  const [isSessionFullscreen, setIsSessionFullscreen] = useState(false);

  // Use WebSocket streaming for real-time session updates
  const { sessions, loading, acknowledgeSession } = useSessionService({
    baseUrl: "http://localhost:8543",
    autoWatch: true, // Enable WebSocket streaming for session list
  });

  // Handle deep linking from notifications - auto-open session from URL
  useEffect(() => {
    const sessionId = searchParams.get("session");
    if (sessionId && sessions.length > 0) {
      const session = sessions.find((s) => s.id === sessionId);
      if (session) {
        setSelectedSession(session);
        setSelectedTab("terminal"); // Always open to terminal for review queue
      }
    }
  }, [searchParams, sessions]);

  const handleSessionClick = (sessionId: string) => {
    // Find the session in the list
    const session = sessions.find((s) => s.id === sessionId);
    if (session) {
      setSelectedSession(session);
      setSelectedTab("terminal"); // Always open to terminal tab for review queue
    }
  };

  const handleSkipSession = async (sessionId: string) => {
    await acknowledgeSession(sessionId);
  };

  const handleCloseSessionDetail = () => {
    // Clear the session query parameter from the URL
    router.push("/review-queue");
    // Close the modal
    setSelectedSession(null);
  };

  return (
    <div className={styles.page}>
      <main className={styles.main}>
        <ReviewQueuePanel
          onSessionClick={handleSessionClick}
          onSkipSession={handleSkipSession}
        />
      </main>

      {/* Session detail modal with terminal view */}
      {selectedSession && (
        <div className={styles.modal} onClick={handleCloseSessionDetail}>
          <div
            className={`${styles.modalContent} ${isSessionFullscreen ? styles.modalContentFullscreen : ""}`}
            onClick={(e) => e.stopPropagation()}
          >
            <SessionDetail
              session={selectedSession}
              onClose={handleCloseSessionDetail}
              onFullscreenChange={setIsSessionFullscreen}
              initialTab="terminal"
            />
          </div>
        </div>
      )}
    </div>
  );
}

export default function ReviewQueuePage() {
  return (
    <Suspense fallback={<div>Loading...</div>}>
      <ReviewQueueContent />
    </Suspense>
  );
}
