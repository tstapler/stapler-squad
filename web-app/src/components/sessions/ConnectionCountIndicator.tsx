"use client";
// +feature: connection-count-indicator

import { useEffect, useRef, useState } from "react";
import * as styles from "./ConnectionCountIndicator.css";

export interface ConnectionCountIndicatorProps {
  /**
   * Current subscriber count for this session's StreamHub, or undefined
   * when unavailable (PathLegacyPerConnection sessions, or before the first
   * connection_count-carrying message has arrived). See
   * useTerminalStream.ts's `connectionCount`.
   */
  count: number | undefined;
  /**
   * True when this tab's last resize vote did not win the hub's negotiated
   * size — surfaces the resize-mismatch sentence in the tooltip
   * (design/ux.md Surface 1, "hover/tap expanded" wireframe). Computed by
   * the caller (TerminalOutput.tsx) by comparing this tab's last requested
   * size against xterm's actual applied dimensions — never speculative
   * (Story 4.2.2 AC3 / UX-AC-10: only shown when a mismatch is real).
   */
  sizeMismatch?: boolean;
}

// design/ux.md's "Rapid count oscillation" edge case (UX-AC-11): debounce the
// value that drives mount/unmount so a flapping reconnect doesn't produce a
// burst of separate mount/announce cycles — mirrors InputDropBadge.tsx's
// episode-coalescing precedent (research/ux.md §3).
const COALESCE_MS = 500;

// design/ux.md's interaction-flow Step 5: the departure ("2 -> 1") must be
// announced, not just silently unmounted — most screen readers only reliably
// announce content *mutations within a still-present* live region, not the
// disappearance of the whole node in the same commit. Hold the departure
// text in the DOM for one announcement cycle before actually unmounting.
const DEPARTURE_HOLD_MS = 1000;

function formatCount(count: number): string {
  return `${count} connection${count === 1 ? "" : "s"} active`;
}

/**
 * Small, non-alarming indicator that another connection (tab, ssq-mux
 * attach, etc.) is attached to the same session (Epic 4.2, Story 4.2.2).
 * Renders only when count > 1 — the overwhelmingly common single-tab case
 * gets zero added visual surface (design/ux.md Surface 1, UX-AC-1).
 *
 * Follows TerminalOutput.tsx's existing reconnecting-banner convention
 * exactly: a single conditionally-mounted `role="status"`/`aria-live="polite"`
 * element, never `role="alert"`. Mounting only on the 1→2 transition (not
 * already true at initial page paint) is what gives "changes-only"
 * announcement semantics for free — a live region's initial-page-load
 * content is not narrated by assistive tech, only content that mutates
 * after the region already exists (Story 4.2.2 AC1).
 *
 * The reverse transition (settled count drops back to <= 1) does NOT unmount
 * immediately: design/ux.md's interaction-flow Step 5 requires an equivalent
 * departure signal ("must not go silent without any signal"), and most
 * screen readers only reliably announce content mutations within a
 * still-present live region, not a node's removal in the same commit. So the
 * departure updates this region's text first, holds it for
 * `DEPARTURE_HOLD_MS`, then unmounts.
 */
export function ConnectionCountIndicator({ count, sizeMismatch }: ConnectionCountIndicatorProps) {
  const [expanded, setExpanded] = useState(false);
  const [settledCount, setSettledCount] = useState(count);
  const [departureLabel, setDepartureLabel] = useState<string | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const departureTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const prevSettledRef = useRef(settledCount);

  useEffect(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
    }
    timerRef.current = setTimeout(() => setSettledCount(count), COALESCE_MS);
    return () => {
      if (timerRef.current) {
        clearTimeout(timerRef.current);
      }
    };
  }, [count]);

  // Detect the settled-count transition back down to <= 1 and hold a
  // departure announcement in the same live region before unmounting
  // (design/ux.md Step 5).
  useEffect(() => {
    const prev = prevSettledRef.current;
    prevSettledRef.current = settledCount;

    const isVisible = settledCount !== undefined && settledCount > 1;
    if (isVisible) {
      // Back to (or still at) a real multi-connection count — cancel any
      // in-flight departure hold so the normal indicator takes over.
      if (departureTimerRef.current) {
        clearTimeout(departureTimerRef.current);
        departureTimerRef.current = null;
      }
      setDepartureLabel(null);
      return;
    }

    const wasVisible = prev !== undefined && prev > 1;
    if (wasVisible) {
      setDepartureLabel(formatCount(1));
      departureTimerRef.current = setTimeout(() => {
        setDepartureLabel(null);
        departureTimerRef.current = null;
      }, DEPARTURE_HOLD_MS);
    }
  }, [settledCount]);

  useEffect(() => {
    return () => {
      if (departureTimerRef.current) {
        clearTimeout(departureTimerRef.current);
      }
    };
  }, []);

  if (departureLabel !== null) {
    // Minimal departure render: same live region, no icon/tooltip — just the
    // text mutation a screen reader can pick up before the node disappears.
    return (
      <span
        className={styles.badge}
        role="status"
        aria-live="polite"
        aria-label={departureLabel}
        data-testid="connection-count-indicator"
      >
        {departureLabel}
      </span>
    );
  }

  if (settledCount === undefined || settledCount <= 1) {
    return null;
  }

  const label = formatCount(settledCount);
  const tooltipText = sizeMismatch
    ? `${label}. Another connection has this session open at a different size.`
    : label;

  return (
    <span
      className={styles.badge}
      role="status"
      aria-live="polite"
      aria-label={label}
      data-testid="connection-count-indicator"
      tabIndex={0}
      onMouseEnter={() => setExpanded(true)}
      onMouseLeave={() => setExpanded(false)}
      onFocus={() => setExpanded(true)}
      onBlur={() => setExpanded(false)}
    >
      <span className={styles.icon} aria-hidden="true">
        👥
      </span>
      <span>{settledCount}</span>
      {expanded && (
        <span className={styles.tooltip} role="tooltip" data-testid="connection-count-tooltip">
          {tooltipText}
        </span>
      )}
    </span>
  );
}
