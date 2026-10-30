"use client";

import { useMemo } from "react";
import { useReviewQueueContext } from "@/lib/contexts/ReviewQueueContext";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { attentionCount } from "./needsHuman";

export const PROSE_DETECTION_FLAG = "attention_prose_question_detection";

/** Shared by ReviewQueueNavBadge, the Sessions-tab badge and the document title. */
export function useAttentionCount(): { count: number; needsHumanOnly: boolean } {
  const { items } = useReviewQueueContext();
  const needsHumanOnly = useFeatureFlag(PROSE_DETECTION_FLAG);
  const count = useMemo(() => attentionCount(items, needsHumanOnly), [items, needsHumanOnly]);
  return { count, needsHumanOnly };
}
