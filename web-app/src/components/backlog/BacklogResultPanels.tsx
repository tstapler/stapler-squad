"use client";

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { TriageResultPanel } from "./TriageResultPanel";
import { ReviewVerdictPanel, latestReviewVerdict } from "./ReviewVerdictPanel";

/** Formatted triage/review results; each panel self-gates on its feature flag and hides when its data is absent. */
export function BacklogResultPanels({ item }: { item: BacklogItem }) {
  const verdict = latestReviewVerdict(item.linkedSessions);
  // Skeleton only while a review session is actually running, not for a review item with no session.
  const reviewRunning = !verdict && item.linkedSessions.some((s) => s.role === "review" && !s.endedAt);
  return (
    <>
      <TriageResultPanel
        triageResult={item.triageResult}
        priority={item.priority}
        loading={item.triageStatus === "running"}
      />
      <ReviewVerdictPanel verdict={verdict} loading={reviewRunning} />
    </>
  );
}
