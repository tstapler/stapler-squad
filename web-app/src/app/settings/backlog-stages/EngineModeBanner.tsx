"use client";

import { useEffect, useRef, useState } from "react";
import { Link } from "@/lib/nav";
import { bannerText, isFullyConfirmedEnforced, reviewLine } from "@/lib/hooks/useEngineModeStatus";
import type { EngineModeView } from "@/lib/hooks/useEngineModeStatus";
import * as styles from "./page.css";

export interface EngineModeBannerProps {
  view: EngineModeView;
  onRetry: () => void;
}

function bannerStyle(view: EngineModeView): string {
  if (view.kind === "loading") return styles.bannerNeutral;
  return isFullyConfirmedEnforced(view) ? styles.bannerSuccess : styles.bannerWarning;
}

/**
 * Says whether Backlog Stages edits are enforced, per caller class. The
 * container has no role (a persistent display must not be announced on first
 * render); changes after the first resolved value go through the one nested,
 * initially empty `role="status"` region (design/ux.md live-region policy).
 */
export function EngineModeBanner({ view, onRetry }: EngineModeBannerProps) {
  const text = bannerText(view);
  const resolved = view.kind !== "loading";
  const lastResolvedTextRef = useRef<string | null>(null);
  const [announcement, setAnnouncement] = useState("");

  useEffect(() => {
    if (!resolved) return;
    if (lastResolvedTextRef.current !== null && lastResolvedTextRef.current !== text) setAnnouncement(text);
    lastResolvedTextRef.current = text;
  }, [resolved, text]);

  // An unwired gate is a definite answer from the server; retrying cannot change it.
  const showRetry = (view.kind === "unknown" && view.gateWired) || view.stale;
  const showIcon = view.kind !== "loading";
  const icon = isFullyConfirmedEnforced(view) ? "✓" : "⚠";

  return (
    <div
      className={bannerStyle(view)}
      data-testid="engine-mode-banner"
      aria-busy={view.kind === "loading" ? true : undefined}
    >
      <div className={styles.bannerBody}>
        <div className={styles.bannerTextRow}>
          {showIcon && (
            <span className={styles.bannerIcon} aria-hidden="true">
              {icon}
            </span>
          )}
          <span data-testid="engine-mode-banner-text">{text}</span>
        </div>
        {view.review && (
          <span
            className={view.review.state === "overdue" ? `${styles.bannerReview} ${styles.bannerReviewOverdue}` : styles.bannerReview}
            data-testid="engine-mode-review"
          >
            {reviewLine(view.review)}
          </span>
        )}
      </div>
      <div className={styles.bannerActions}>
        {showRetry && (
          // analytics-exempt
          <button type="button" className={styles.bannerBtn} onClick={onRetry} data-testid="engine-mode-banner-retry">
            Retry
          </button>
        )}
        {resolved && (
          <Link href={`/settings/features#${view.flagRowId}`} className={styles.bannerBtn} data-testid="engine-mode-banner-flags-link">
            Change in Feature Flags
          </Link>
        )}
      </div>
      <div role="status" className={styles.visuallyHidden} data-testid="engine-mode-banner-live">
        {announcement}
      </div>
    </div>
  );
}
