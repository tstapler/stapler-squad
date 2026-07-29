"use client";
// +feature: input-drop-badge

import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { LiveRegion } from "@/components/ui/LiveRegion";
import { DEFAULT_TOAST_MS } from "@/lib/notification-policy";
import * as styles from "./InputDropBadge.css";

export interface InputDropBadgeProps {
  /** Number of dropped keystrokes in the most recent coalesced episode. */
  count: number;
  /**
   * Monotonically increasing episode identifier — bumped by the caller once
   * per `useDropEpisodeCoalescer` `onFlush` (Task 2.3.3a), regardless of
   * whether `count` itself changed. This is what lets two consecutive
   * episodes with an identical `count` (e.g. "1" then "1" again) still
   * produce a distinct DOM text mutation on the live region (design/ux.md
   * §Step 4 item 4 / UX-AC-7) — `count` alone can't disambiguate that case
   * when the visual badge never fully leaves the screen between episodes
   * (design/ux.md §2.3 Case C).
   */
  episodeSeq: number;
}

function formatBadgeText(count: number): string {
  return `${count} keystroke${count === 1 ? "" : "s"} dropped — reconnecting`;
}

/**
 * Portal-rendered, auto-dismissing badge that visibly and audibly announces
 * a `DropEpisode` (Story 2.3). Modeled on `XtermTerminal.tsx`'s `copiedToast`
 * pattern — see `.claude/rules/css-architecture.md` for the vanilla-extract
 * convention used by `InputDropBadge.css.ts`.
 *
 * The nested `LiveRegion` is unconditionally rendered (never gated on the
 * visual badge's visibility) — see design/ux.md §2.4: a stable, always-
 * present live-region DOM node is what assistive technology reliably detects
 * content mutations on.
 */
export function InputDropBadge({ count, episodeSeq }: InputDropBadgeProps) {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    // design/ux.md §3.3 — defensive no-op on count <= 0 (should not happen
    // given MessageQueue.close()'s contract, but a normal user-initiated
    // disconnect with an empty queue should never surface a badge).
    if (count <= 0) {
      return;
    }

    // A new episode always (re)shows the badge and restarts its own
    // auto-dismiss hold from this episode's close time — it does not
    // inherit/extend a prior episode's remaining hold time
    // (design/ux.md §2.3 Case C).
    //
    // No manual dedup ref here: the `[episodeSeq, count]` dependency array
    // already guarantees this effect only re-fires when a genuinely new
    // episode arrives, and this effect's own cleanup (returned below) is
    // what clears the previous episode's timer — including under React
    // StrictMode's dev-only setup→cleanup→setup replay on mount, where a
    // separate cleanup-only effect would otherwise clear the timer armed by
    // setup #1 with no corresponding rearm in setup #2.
    setVisible(true);

    const dismissTimer = setTimeout(() => {
      setVisible(false);
    }, DEFAULT_TOAST_MS);

    return () => {
      clearTimeout(dismissTimer);
    };
  }, [episodeSeq, count]);

  if (typeof document === "undefined") {
    return null;
  }

  const badgeText = count > 0 ? formatBadgeText(count) : "";

  // Task 2.3.2 / adversarial-review.md dedup fix: append an invisible
  // (zero-width-space) nonce derived from episodeSeq so two consecutive
  // episodes with an identical human-readable count still produce a
  // distinct underlying text node — `aria-live` only fires on a genuine DOM
  // text mutation, and React bails on a redundant identical-string update.
  const announcementText = badgeText
    ? `${badgeText}${"​".repeat(episodeSeq % 2)}`
    : "";

  return createPortal(
    <>
      {visible && count > 0 && (
        <div
          className={styles.badge}
          data-testid="input-drop-badge"
          // Decorative only — the LiveRegion below is the sole announcement
          // channel, so this element must not be independently read by AT.
          aria-hidden="true"
        >
          ⚠ {badgeText}
        </div>
      )}
      <LiveRegion role="alert" politeness="assertive" message={announcementText} />
    </>,
    document.body
  );
}
