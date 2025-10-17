"use client";

import { useState } from "react";
import { Session } from "@/gen/session/v1/types_pb";
import { ReviewQueuePanel } from "@/components/sessions/ReviewQueuePanel";
import { SessionDetail, SessionDetailTab } from "@/components/sessions/SessionDetail";
import { useSessionService } from "@/lib/hooks/useSessionService";
import styles from "./page.module.css";

export default function ReviewQueuePage() {
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);
  const [selectedTab, setSelectedTab] = useState<SessionDetailTab>("terminal");
  const [isSessionFullscreen, setIsSessionFullscreen] = useState(false);

  // Use WebSocket streaming for real-time session updates
  const { sessions, loading, acknowledgeSession } = useSessionService({
    baseUrl: "http://localhost:8543",
    autoWatch: true, // Enable WebSocket streaming for session list
  });

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
        <div className={styles.modal} onClick={() => setSelectedSession(null)}>
          <div
            className={`${styles.modalContent} ${isSessionFullscreen ? styles.modalContentFullscreen : ""}`}
            onClick={(e) => e.stopPropagation()}
          >
            <SessionDetail
              session={selectedSession}
              onClose={() => setSelectedSession(null)}
              onFullscreenChange={setIsSessionFullscreen}
              initialTab="terminal"
            />
          </div>
        </div>
      )}
    </div>
  );
}
