"use client";
// +feature: review-queue session-approval session-triage

import { useState, useEffect, useRef, useCallback, useMemo, Suspense } from "react";
import { usePageView } from "@/lib/analytics/usePageView";
import { useSearchParams, useRouter } from "next/navigation";
import { Session, SessionSchema, SessionStatus, ReviewItem } from "@/gen/session/v1/types_pb";
import { create } from "@bufbuild/protobuf";
import { ReviewQueuePanel } from "@/components/sessions/ReviewQueuePanel";
import { SessionDetail, SessionDetailTab } from "@/components/sessions/SessionDetail";
import { useSessionServiceContext } from "@/lib/contexts/SessionServiceContext";
import { useReviewQueueContext } from "@/lib/contexts/ReviewQueueContext";
import { useWatchBacklogItems } from "@/lib/hooks/useWatchBacklogItems";
import { getAvailableActions } from "@/lib/backlog/itemActions";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { useKeyboard } from "@/lib/hooks/useKeyboard";
import { KeyboardHints } from "@/components/ui/KeyboardHint";
import * as styles from "./page.css";

// Stable reference — useWatchBacklogItems only reruns its connection effects when the
// joined filter key changes, but a fresh array literal every render is still bad hygiene.
const PLAN_REVIEW_STATUS_FILTER = ["ready", "queued"];

// Construct a minimal Session from ReviewItem data for immediate modal opening
// before the session list has finished loading.
function sessionFromReviewItem(item: ReviewItem): Session {
  return create(SessionSchema, {
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
  const [isHelpOpen, setIsHelpOpen] = useState(false);

  // Auto-advance preference (default: on), persisted to localStorage
  const [autoAdvance, setAutoAdvance] = useState<boolean>(() => {
    if (typeof window === "undefined") return true;
    const stored = localStorage.getItem("review-queue-auto-advance");
    return stored === null ? true : stored === "true";
  });
  const autoAdvanceRef = useRef(autoAdvance);
  useEffect(() => { autoAdvanceRef.current = autoAdvance; }, [autoAdvance]);

  // Ref for focus trap inside the session-detail modal
  const modalContentRef = useRef<HTMLDivElement>(null);

  // Use the global session service context — avoids a competing WebSocket stream
  const { sessions, runOneShot } = useSessionServiceContext();

  // Backlog items whose plan is awaiting the user's approval — surfaced here so "things
  // needing you" aren't scattered across the board/stuck-items page too.
  const { items: backlogItems } = useWatchBacklogItems({ statusFilter: PLAN_REVIEW_STATUS_FILTER });
  const planReviewItems = useMemo(
    () => backlogItems.filter((item) => getAvailableActions(item).actions.has("approve_plan")),
    [backlogItems]
  );

  // S3-3: Adapter from RunOneShotResponse to the shape ReviewQueuePanel expects
  const handleRunOneShot = useCallback(
    async (sessionId: string, prompt: string) => {
      const response = await runOneShot(sessionId, prompt, 0);
      if (!response) return null;
      return { prUrl: response.prUrl || undefined, error: response.error || undefined };
    },
    [runOneShot]
  );

  // Acknowledge function for dismissing sessions from the modal.
  // allQueueItems is the unfiltered Redux store list — used as the existence oracle in the
  // "deleted externally" effect below so that a status transition to ACTIVE/PROCESSING
  // (which filters the session from the visible queue) does not spuriously trigger auto-advance.
  const { acknowledgeSession, items: allQueueItems } = useReviewQueueContext();

  // Review queue items for navigation (next/previous)
  const [reviewQueueItems, setReviewQueueItems] = useState<Session[]>([]);
  // Full ReviewItem data for fallback session construction before sessions load
  const [queueItems, setQueueItems] = useState<ReviewItem[]>([]);

  // Sessions that are simply busy (creating/active) rather than waiting on the user — shown
  // separately so "needs you" (the queue below) and "still working, leave it" don't blur
  // together into one undifferentiated list.
  const attentionSessionIds = useMemo(
    () => new Set(reviewQueueItems.map((s) => s.id)),
    [reviewQueueItems]
  );
  const workingSessions = useMemo(
    () =>
      sessions.filter(
        (s) =>
          (s.status === SessionStatus.ACTIVE || s.status === SessionStatus.CREATING) &&
          !attentionSessionIds.has(s.id)
      ),
    [sessions, attentionSessionIds]
  );

  // Refs to avoid stale closures inside setTimeout callbacks
  const reviewQueueItemsRef = useRef<Session[]>([]);
  const selectedSessionRef = useRef<Session | null>(null);

  useEffect(() => { reviewQueueItemsRef.current = reviewQueueItems; }, [reviewQueueItems]);
  useEffect(() => { selectedSessionRef.current = selectedSession; }, [selectedSession]);

  // Trap focus inside the session-detail modal while it is open
  useFocusTrap(modalContentRef, !!selectedSession);

  // Global keyboard shortcuts for this page
  useKeyboard({
    "?": () => setIsHelpOpen((v) => !v),
    Escape: () => {
      if (isHelpOpen) {
        setIsHelpOpen(false);
      } else if (selectedSession) {
        handleCloseSessionDetail();
      }
    },
  });

  // Handle deep linking from notifications - auto-open session from URL.
  // Uses queueItems as fallback so the modal opens even before the session list loads.
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
    // always opens immediately regardless of whether the session list has loaded.
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
  const handleAutoAdvance = useCallback((resolvedSessionId?: string, force = false) => {
    setTimeout(() => {
      if (!force && !autoAdvanceRef.current) return; // Auto-advance disabled by user preference

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
    handleAutoAdvance(current.id, true); // explicit dismiss always advances regardless of auto-advance setting
  }, [acknowledgeSession, handleAutoAdvance]);

  // Auto-advance when the currently selected session is deleted externally (not via dismiss/acknowledge).
  // Uses allQueueItems (the unfiltered Redux store) rather than reviewQueueItems (visible filtered list)
  // so that a session transitioning to ACTIVE/PROCESSING — which is filtered from the visible queue but
  // remains in the store — does not incorrectly trigger auto-advance.
  // reviewQueueItems is kept as the dep so the effect fires when the visible queue changes (the moment
  // we need to re-evaluate), but the guard checks allQueueItems to distinguish "filtered out" from "removed".
  // force=false so the user's auto-advance preference is respected even on genuine removals (e.g. after
  // approving/denying a permission request — the user may still want to watch the session continue).
  useEffect(() => {
    if (!selectedSession) return;
    const stillInQueue = allQueueItems.some((item) => item.sessionId === selectedSession.id);
    if (!stillInQueue) {
      handleAutoAdvance(selectedSession.id);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reviewQueueItems]);

  // Queue position for the header badge ("2 of 5")
  const queuePosition = selectedSession
    ? reviewQueueItems.findIndex((s) => s.id === selectedSession.id) + 1
    : 0;
  const queueTotal = reviewQueueItems.length;

  return (
    <div className={styles.page}>
      <div id="main-content" className={styles.main}>
        {planReviewItems.length > 0 && (
          <section className={styles.inlineSection} aria-label="Plan reviews" data-testid="plan-review-section">
            <h3 className={styles.inlineSectionTitle}>
              Plan Reviews
              <span className={styles.inlineSectionCount}>{planReviewItems.length}</span>
            </h3>
            <div className={styles.inlineSectionList}>
              {planReviewItems.map((item) => (
                <button
                  key={item.id}
                  className={styles.inlineSectionRow}
                  onClick={() => router.push(`/backlog?item=${item.id}`)}
                  data-testid={`plan-review-item-${item.id}`}
                >
                  <span>{item.title}</span>
                  <span className={styles.inlineSectionRowMeta}>Plan awaiting approval</span>
                </button>
              ))}
            </div>
          </section>
        )}

        {workingSessions.length > 0 && (
          <section className={styles.inlineSection} aria-label="Currently working sessions" data-testid="working-sessions-section">
            <h3 className={styles.inlineSectionTitle}>
              Currently Working
              <span className={styles.inlineSectionCount}>{workingSessions.length}</span>
            </h3>
            <div className={styles.inlineSectionList}>
              {workingSessions.map((s) => (
                <button
                  key={s.id}
                  className={styles.inlineSectionRow}
                  onClick={() => handleSessionClick(s.id)}
                  data-testid={`working-session-${s.id}`}
                >
                  <span>{s.title}</span>
                  <span className={styles.inlineSectionRowMeta}>
                    {s.status === SessionStatus.CREATING ? "Queued" : "In progress"} — nothing to do yet
                  </span>
                </button>
              ))}
            </div>
          </section>
        )}

        <ReviewQueuePanel
          onSessionClick={handleSessionClick}
          onItemsChange={handleItemsChange}
          onAcknowledged={handleAcknowledged}
          onRunOneShot={handleRunOneShot}
          autoAdvance={autoAdvance}
          onAutoAdvanceChange={(val) => {
            setAutoAdvance(val);
            localStorage.setItem("review-queue-auto-advance", String(val));
          }}
        />
      </div>

      {/* Session detail modal with terminal view */}
      {selectedSession && (
        <div className={styles.modal} onClick={handleCloseSessionDetail}>
          <div
            ref={modalContentRef}
            className={`${styles.modalContent} ${isSessionFullscreen ? styles.modalContentFullscreen : ""}`}
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
            aria-label={selectedSession.title}
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

      {/* Keyboard shortcuts help overlay */}
      {isHelpOpen && (
        <div className={styles.helpOverlay} onClick={() => setIsHelpOpen(false)}>
          <div
            className={styles.helpOverlayContent}
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
            aria-labelledby="review-queue-help-title"
          >
            <div className={styles.helpOverlayHeader}>
              <h2 id="review-queue-help-title">Keyboard Shortcuts</h2>
              <button
                className={styles.helpOverlayCloseButton}
                onClick={() => setIsHelpOpen(false)}
                aria-label="Close keyboard shortcuts"
              >
                ✕
              </button>
            </div>
            <KeyboardHints
              hints={[
                { keys: "?", description: "Show / hide this help" },
                { keys: "Escape", description: "Close dialog or modal" },
                { keys: "Enter", description: "Approve pending request (when 1 approval visible)" },
                { keys: ["Shift", "Enter"], description: "Deny pending request (when 1 approval visible)" },
                { keys: "]", description: "Next queue item" },
                { keys: "[", description: "Previous queue item" },
                { keys: ["Shift", "→"], description: "Next session (in modal)" },
                { keys: ["Shift", "←"], description: "Previous session (in modal)" },
              ]}
            />
          </div>
        </div>
      )}

      {/* Floating help button */}
      <button
        className={styles.helpButton}
        onClick={() => setIsHelpOpen(true)}
        aria-label="Show keyboard shortcuts"
        title="Keyboard shortcuts (?)"
      >
        ?
      </button>
    </div>
  );
}

function ReviewQueueSkeleton() {
  return (
    <div className={styles.page}>
      <div id="main-content" className={styles.main} aria-busy="true" aria-label="Loading review queue">
        <div className={styles.skeletonHeader} />
        <div className={styles.skeletonList}>
          {[1, 2, 3].map((i) => (
            <div key={i} className={styles.skeletonCard} aria-hidden="true" />
          ))}
        </div>
      </div>
    </div>
  );
}

export default function ReviewQueuePage() {
  usePageView();
  return (
    <Suspense fallback={<ReviewQueueSkeleton />}>
      <ReviewQueueContent />
    </Suspense>
  );
}
