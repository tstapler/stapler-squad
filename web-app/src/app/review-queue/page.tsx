"use client";

import { useState, useEffect, Suspense } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { Session, ReviewItem } from "@/gen/session/v1/types_pb";
import { ReviewQueuePanel } from "@/components/sessions/ReviewQueuePanel";
import { SessionDetail, SessionDetailTab } from "@/components/sessions/SessionDetail";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { getApiBaseUrl } from "@/lib/config";
import styles from "./page.module.css";

// Construct a minimal Session from ReviewItem data for immediate modal opening
// before useSessionService has finished loading.
function sessionFromReviewItem(item: ReviewItem): Session {
  return new Session({
    id: item.sessionId,
    title: item.sessionName,
    path: item.path,
    workingDir: item.workingDir,
    branch: item.branch,
    status: item.status,
    program: item.program,
    tags: item.tags,
  });
}

function ReviewQueueContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);
  const [selectedTab, setSelectedTab] = useState<SessionDetailTab>("terminal");
  const [isSessionFullscreen, setIsSessionFullscreen] = useState(false);

  // Use WebSocket streaming for real-time session updates
  const { sessions } = useSessionService({
    baseUrl: getApiBaseUrl(),
    autoWatch: true, // Enable WebSocket streaming for session list
  });

  // Review queue items for navigation (next/previous)
  const [reviewQueueItems, setReviewQueueItems] = useState<Session[]>([]);
  // Full ReviewItem data for fallback session construction before sessions load
  const [queueItems, setQueueItems] = useState<ReviewItem[]>([]);

  // Handle deep linking from notifications - auto-open session from URL.
  // Uses queueItems as fallback so the modal opens even before useSessionService loads.
  useEffect(() => {
    const sessionId = searchParams.get("session");
    if (!sessionId) return;
    const fromSessions = sessions.find((s) => s.id === sessionId);
    const fromQueue = queueItems.find((i) => i.sessionId === sessionId);
    const session = fromSessions ?? (fromQueue ? sessionFromReviewItem(fromQueue) : undefined);
    if (session) {
      setSelectedSession(session);
      setSelectedTab("terminal");
    }
  }, [searchParams, sessions, queueItems]);

  const handleSessionClick = (sessionId: string) => {
    // Try full session data first; fall back to queue item data so the modal
    // always opens immediately regardless of whether useSessionService has loaded.
    const fromSessions = sessions.find((s) => s.id === sessionId);
    const fromQueue = queueItems.find((i) => i.sessionId === sessionId);
    const session = fromSessions ?? (fromQueue ? sessionFromReviewItem(fromQueue) : undefined);
    if (session) {
      setSelectedSession(session);
      setSelectedTab("terminal");
    }
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
          onItemsChange={(items) => {
            setQueueItems(items);
            // Build navigation list using full session data when available,
            // falling back to queue item data so navigation works before sessions load.
            const queueSessions = items.map(
              (item) => sessions.find((s) => s.id === item.sessionId) ?? sessionFromReviewItem(item)
            );
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
              key={selectedSession.id}
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
