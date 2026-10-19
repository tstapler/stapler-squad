"use client";

import { useMemo, useRef } from "react";
import { useBackgroundTelemetry } from "@/lib/hooks/useBackgroundTelemetry";
import { useFeatureFlagValue } from "@/lib/hooks/useFeatureFlagValue";
import { useBackgroundSessions, type BackgroundSessionsState } from "@/lib/hooks/useBackgroundSessions";
import {
  buildActivityRows,
  buildRollup,
  type ActivityRow,
  type BackgroundRollup,
} from "@/lib/utils/backgroundActivity";
import { knownHiddenSessions } from "@/lib/utils/hiddenSessionRegistry";
import type { NotificationHistoryItem } from "@/lib/types/notification";

export const BACKGROUND_FEED_FLAG = "background_activity_feed";

export interface BackgroundActivityState {
  rows: ActivityRow[];
  rollup: BackgroundRollup;
  /** Hidden sessions needing a human or reporting failure; the tab's badge and accessible name. */
  exceptionCount: number;
  /** At least one hidden session is known (live list or seen earlier in this tab). */
  hasHiddenSessions: boolean;
  /** Poll state, passed through unchanged so wrappers can add flag gating, telemetry and error UI. */
  poll: BackgroundSessionsState;
  /** `background_activity_feed`; off means exception rows only and the legacy empty state. */
  feedEnabled: boolean;
}

/**
 * The Background tab's data: polls hidden sessions (only while `enabled`) and joins them
 * with notification history into activity rows plus a roll-up. The panel composes; the
 * row logic lives in `backgroundActivity.ts`.
 */
export function useBackgroundActivity(
  enabled: boolean,
  history: readonly NotificationHistoryItem[],
  onEvidenceAlert?: () => void,
): BackgroundActivityState {
  const feedEnabled = useFeatureFlagValue(BACKGROUND_FEED_FLAG, undefined, true);
  const poll = useBackgroundSessions(enabled);
  const { sessions, departed, lastUpdatedAt } = poll;

  // Rows of the previous poll feed `departed`; advancing only when a new poll lands keeps
  // a departed row for one full poll cycle even if history re-renders in between.
  const cycle = useRef<{ pollAt: number | null; previous: ActivityRow[]; current: ActivityRow[] }>({
    pollAt: null,
    previous: [],
    current: [],
  });

  const rows = useMemo(() => {
    if (cycle.current.pollAt !== lastUpdatedAt) {
      cycle.current = { pollAt: lastUpdatedAt, previous: cycle.current.current, current: [] };
    }
    const next = buildActivityRows(history, sessions, {
      previous: cycle.current.previous,
      departed: lastUpdatedAt === null ? knownHiddenSessions() : departed,
      exceptionsOnly: !feedEnabled,
    });
    cycle.current.current = next;
    return next;
  }, [history, sessions, departed, lastUpdatedAt, feedEnabled]);

  const rollup = useMemo(() => buildRollup(rows), [rows]);
  useBackgroundTelemetry(rows, lastUpdatedAt, enabled && feedEnabled, onEvidenceAlert);

  return {
    rows,
    rollup,
    exceptionCount: rollup.exceptions,
    hasHiddenSessions: sessions.length > 0 || departed.length > 0,
    poll,
    feedEnabled,
  };
}
