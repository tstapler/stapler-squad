"use client";

import { useState, useEffect, useRef, useCallback, Suspense } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { Session, ReviewItem } from "@/gen/session/v1/types_pb";
import { ReviewQueuePanel } from "@/components/sessions/ReviewQueuePanel";
import { SessionDetail, SessionDetailTab } from "@/components/sessions/SessionDetail";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { useReviewQueueContext } from "@/lib/contexts/ReviewQueueContext";
import { useAuth } from "@/lib/contexts/AuthContext";
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
  const { authEnabled, authenticated, loading: authLoading } = useAuth();
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);
  const [selectedTab, setSelectedTab] = useState<SessionDetailTab>("terminal");
  const [isSessionFullscreen, setIsSessionFullscreen] = useState(false);

  // Use WebSocket streaming for real-time session updates
  const { sessions } = useSessionService({
    baseUrl: getApiBaseUrl(),
    autoWatch: true, // Enable WebSocket streaming for session list
    enabled: !authLoading && (!authEnabled || authenticated),
  });

  // Acknowledge function for dismissing sessions from the modal
  const { acknowledgeSession } = useReviewQueueContext();

  // Review queue items for navigation (next/previous)
  const [reviewQueueItems, setReviewQueueItems] = useState<Session[]>([]);
  // Full ReviewItem data for fallback session construction before sessions load
  const [queueItems, setQueueItems] = useState<ReviewItem[]>([]);

  // Refs to avoid stale closures inside setTimeout callbacks
  const reviewQueueItemsRef = useRef<Session[]>([]);
  const selectedSessionRef = useRef<Session | null>(null);

  useEffect(() => { reviewQueueItemsRef.current = reviewQueueItems; }, [reviewQueueItems]);
  useEffect(() => { selectedSessionRef.current = selectedSession; }, [selectedSession]);

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

  // Stable callback for ReviewQueuePanel to report items — avoids infinite render loop.
  // Separating the queueSessions computation into its own effect prevents a re-render
  // cycle where an inline onItemsChange reference change triggers the panel's useEffect,
  // which calls setReviewQueueItems with a new array, which triggers a parent re-render,
  // which creates a new onItemsChange reference… blocking Next.js navigation forever.
  const handleItemsChange = useCallback((incomingItems: ReviewItem[]) => {
    setQueueItems(incomingItems);
  }, []);

  // Recompute reviewQueueItems whenever queueItems or sessions change.
  useEffect(() => {
    const queueSessions = queueItems.map(
      (item) => sessions.find((s) => s.id === item.sessionId) ?? sessionFromReviewItem(item)
    );
    setReviewQueueItems(queueSessions);
  }, [queueItems, sessions]);

  // Auto-advance to the next queue item after resolving the current one.
  // resolvedSessionId: the session that was just resolved (exclude from next-item search
  //   to handle the race where WebSocket hasn't removed it yet).
  const handleAutoAdvance = useCallback((resolvedSessionId?: string) => {
    setTimeout(() => {
      const currentItems = reviewQueueItemsRef.current;
      const currentSelected = selectedSessionRef.current;

      // Exclude the just-resolved session to avoid advancing to it again
      const remainingItems = resolvedSessionId
        ? currentItems.filter((s) => s.id !== resolvedSessionId)
        : currentItems;

      if (remainingItems.length === 0) {
        // Queue is empty — close modal and let the completion state show
        router.push("/review-queue");
        setSelectedSession(null);
        return;
      }

      if (!currentSelected) return;

      const currentIdx = remainingItems.findIndex((s) => s.id === currentSelected.id);

      if (currentIdx !== -1) {
        // Current session is still in the queue; advance to the next one (circular)
        const nextIdx = (currentIdx + 1) % remainingItems.length;
        const next = remainingItems[nextIdx];
        setSelectedSession(next);
        router.push(`/review-queue?session=${next.id}`);
      } else {
        // Current session was removed — navigate to the item at the same position
        const resolvedIdx = resolvedSessionId
          ? currentItems.findIndex((s) => s.id === resolvedSessionId)
          : 0;
        const targetIdx = Math.min(Math.max(resolvedIdx, 0), remainingItems.length - 1);
        const next = remainingItems[targetIdx];
        setSelectedSession(next);
        router.push(`/review-queue?session=${next.id}`);
      }
    }, 300);
  }, [router]);

  // Called when the user acknowledges a session from the queue list while the modal is open.
  // Only triggers auto-advance if it's the currently selected session being dismissed.
  const handleAcknowledged = useCallback((sessionId: string) => {
    if (selectedSessionRef.current?.id === sessionId) {
      handleAutoAdvance(sessionId);
    }
  }, [handleAutoAdvance]);

  // Called when the user clicks the dismiss button in the session detail modal.
  // Acknowledges the current session and auto-advances to the next queue item.
  const handleDismissFromQueue = useCallback(async () => {
    const current = selectedSessionRef.current;
    if (!current) return;
    await acknowledgeSession(current.id);
    handleAutoAdvance(current.id);
  }, [acknowledgeSession, handleAutoAdvance]);

  // Queue position for the header badge ("2 of 5")
  const queuePosition = selectedSession
    ? reviewQueueItems.findIndex((s) => s.id === selectedSession.id) + 1
    : 0;
  const queueTotal = reviewQueueItems.length;

  return (
    <div className={styles.page}>
      <main id="main-content" className={styles.main}>
        <ReviewQueuePanel
          onSessionClick={handleSessionClick}
          onItemsChange={handleItemsChange}
          onAcknowledged={handleAcknowledged}
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
              onApprovalResolved={() => handleAutoAdvance(selectedSession.id)}
              onDismissFromQueue={handleDismissFromQueue}
              queuePosition={queuePosition}
              queueTotal={queueTotal}
            />
          </div>
        </div>
      )}
    </div>
  );
}

function ReviewQueueSkeleton() {
  return (
    <div className={styles.page}>
      <main id="main-content" className={styles.main} aria-busy="true" aria-label="Loading review queue">
        <div className={styles.skeletonHeader} />
        <div className={styles.skeletonList}>
          {[1, 2, 3].map((i) => (
            <div key={i} className={styles.skeletonCard} aria-hidden="true" />
          ))}
        </div>
      </main>
    </div>
  );
}

export default function ReviewQueuePage() {
  return (
    <Suspense fallback={<ReviewQueueSkeleton />}>
      <ReviewQueueContent />
    </Suspense>
  );
}
