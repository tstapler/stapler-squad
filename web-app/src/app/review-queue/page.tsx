"use client";

import { useState, useEffect, Suspense } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { Session } from "@/gen/session/v1/types_pb";
import { ReviewQueuePanel } from "@/components/sessions/ReviewQueuePanel";
import { SessionDetail, SessionDetailTab } from "@/components/sessions/SessionDetail";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { getApiBaseUrl } from "@/lib/config";
import styles from "./page.module.css";

function ReviewQueueContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);
  const [selectedTab, setSelectedTab] = useState<SessionDetailTab>("terminal");
  const [isSessionFullscreen, setIsSessionFullscreen] = useState(false);

  // Use WebSocket streaming for real-time session updates
  const { sessions, loading } = useSessionService({
    baseUrl: getApiBaseUrl(),
    autoWatch: true, // Enable WebSocket streaming for session list
  });

  // Get review queue items (sessions that need attention, sorted by priority)
  const [reviewQueueItems, setReviewQueueItems] = useState<Session[]>([]);

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
    // Try to open immediately if sessions are already loaded
    const session = sessions.find((s) => s.id === sessionId);
    if (session) {
      setSelectedSession(session);
      setSelectedTab("terminal"); // Always open to terminal tab for review queue
    }
    // Always update URL - the useEffect will open the modal when sessions finish loading
    router.push(`/review-queue?session=${sessionId}`);
  };

  // Navigate to next session in review queue
  const handleNextSession = () => {
    if (!selectedSession || reviewQueueItems.length === 0) return;

    const currentIndex = reviewQueueItems.findIndex((s) => s.id === selectedSession.id);
    const nextIndex = (currentIndex + 1) % reviewQueueItems.length;
    const nextSession = reviewQueueItems[nextIndex];

    setSelectedSession(nextSession);
    router.push(`/review-queue?session=${nextSession.id}`);
  };

  // Navigate to previous session in review queue
  const handlePreviousSession = () => {
    if (!selectedSession || reviewQueueItems.length === 0) return;

    const currentIndex = reviewQueueItems.findIndex((s) => s.id === selectedSession.id);
    const previousIndex = currentIndex === 0 ? reviewQueueItems.length - 1 : currentIndex - 1;
    const previousSession = reviewQueueItems[previousIndex];

    setSelectedSession(previousSession);
    router.push(`/review-queue?session=${previousSession.id}`);
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
          onItemsChange={(sessionIds) => {
            // Update the review queue items for navigation
            const queueSessions = sessionIds
              .map((id) => sessions.find((s) => s.id === id))
              .filter((s): s is Session => s !== undefined);
            setReviewQueueItems(queueSessions);
          }}
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
              showNavigation={reviewQueueItems.length > 1}
              onNext={handleNextSession}
              onPrevious={handlePreviousSession}
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
