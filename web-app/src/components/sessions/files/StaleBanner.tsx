"use client";

import type { StaleAction, StaleInfo } from "@/lib/hooks/useBufferFreshness";
import { BannerItem, type BannerSpec } from "./BannerStack";
import { bannerAction } from "./StaleBanner.css";

/** The stale-on-disk banner as a stack entry; "stale with edits" outranks a clean stale buffer. */
export function staleBannerSpec(stale: StaleInfo, sessionRunning: boolean, onAction: (action: StaleAction) => void): BannerSpec {
  const suffix = sessionRunning ? " (session is running; this may be the agent)" : " (source unknown)";
  return {
    id: `stale:${stale.path}`,
    kind: stale.dirty ? "stale_dirty" : "stale_clean",
    seq: 1,
    testId: "files-stale-banner",
    onDismiss: () => onAction("dismiss"),
    body: (
      <>
        Changed on disk -{" "}
        {stale.dirty ? (
          <button type="button" className={bannerAction} onClick={() => onAction("review")}>review diff</button>
        ) : (
          <button type="button" className={bannerAction} onClick={() => onAction("reload")}>Reload</button>
        )}
        {suffix}
      </>
    ),
  };
}

interface StaleBannerProps {
  stale: StaleInfo;
  sessionRunning: boolean;
  onAction: (action: StaleAction) => void;
}

/** A status line, never a dialog: it must not take focus from wherever the user is typing. */
export function StaleBanner({ stale, sessionRunning, onAction }: StaleBannerProps) {
  return <BannerItem spec={staleBannerSpec(stale, sessionRunning, onAction)} />;
}
