"use client";

import type { StaleAction, StaleInfo } from "@/lib/hooks/useBufferFreshness";
import { banner, bannerAction } from "./StaleBanner.css";

interface StaleBannerProps {
  stale: StaleInfo;
  sessionRunning: boolean;
  onAction: (action: StaleAction) => void;
}

/** A status line, never a dialog: it must not take focus from wherever the user is typing. */
export function StaleBanner({ stale, sessionRunning, onAction }: StaleBannerProps) {
  const suffix = sessionRunning ? " (session is running; this may be the agent)" : " (source unknown)";
  return (
    <div className={banner} role="status" data-testid="files-stale-banner">
      <span>
        Changed on disk -{" "}
        {stale.dirty ? (
          <button type="button" className={bannerAction} onClick={() => onAction("review")}>review diff</button>
        ) : (
          <button type="button" className={bannerAction} onClick={() => onAction("reload")}>Reload</button>
        )}
        {suffix}
      </span>
      <button type="button" className={bannerAction} aria-label="Dismiss" onClick={() => onAction("dismiss")}>×</button>
    </div>
  );
}
