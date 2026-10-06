"use client";

import { CollapsibleSection } from "@/components/ui/Collapsible";
import type { ReviewFeedbackSummary } from "@/lib/vcs/types";
import * as styles from "./VcsWidgetReviewFeedback.css";

interface VcsWidgetReviewFeedbackProps {
  reviewFeedback: ReviewFeedbackSummary[];
}

export function VcsWidgetReviewFeedback({ reviewFeedback }: VcsWidgetReviewFeedbackProps) {
  const blockingReviews = reviewFeedback.filter((review) => review.state === "CHANGES_REQUESTED");
  if (blockingReviews.length === 0) return null;

  return (
    <CollapsibleSection
      sectionKey="review-feedback"
      title="Review feedback"
      defaultExpanded={false}
    >
      <ul className={styles.list}>
        {blockingReviews.map((review, index) => (
          <li key={`${review.author}-${index}`} className={styles.review}>
            <span className={styles.author}>{review.author}</span>
            {/* Plain JSX text interpolation — auto-escaped, XSS-safe. Do not switch to dangerouslySetInnerHTML for Markdown rendering without a sanitizing renderer. */}
            <p className={styles.body}>{review.body}</p>
          </li>
        ))}
      </ul>
    </CollapsibleSection>
  );
}
