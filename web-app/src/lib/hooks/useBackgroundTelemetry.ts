"use client";

import { useContext, useEffect, useRef } from "react";
import { AnalyticsContext } from "@/lib/contexts/AnalyticsContext";
import type { ActivityRow } from "@/lib/utils/backgroundActivity";
import {
  createEmptyEvidenceAlert,
  stateCountLabels,
  stateCounts,
  type EvidenceAlertState,
} from "@/lib/utils/backgroundTelemetry";

const ALERT_KEY = "ssq.background.evidenceAlert";

function loadAlertState(): EvidenceAlertState | undefined {
  try {
    const raw = window.localStorage.getItem(ALERT_KEY);
    return raw ? (JSON.parse(raw) as EvidenceAlertState) : undefined;
  } catch {
    return undefined;
  }
}

function saveAlertState(state: EvidenceAlertState) {
  try {
    window.localStorage.setItem(ALERT_KEY, JSON.stringify(state));
  } catch {
    // The window then restarts with each page load; an unsaved alert state is never an error.
  }
}

/**
 * Once per landed poll: posts `background_activity_rows` (counts by state as labels) to
 * `/api/analytics` and feeds the empty-evidence alert. Inert without an analytics provider.
 */
export function useBackgroundTelemetry(
  rows: readonly ActivityRow[],
  pollAt: number | null,
  enabled: boolean,
  onEvidenceAlert?: () => void,
) {
  const analytics = useContext(AnalyticsContext);
  const lastPoll = useRef<number | null>(null);
  const notifyRef = useRef(onEvidenceAlert);
  notifyRef.current = onEvidenceAlert;
  const alertRef = useRef<ReturnType<typeof createEmptyEvidenceAlert> | null>(null);

  useEffect(() => {
    if (!enabled || pollAt === null || lastPoll.current === pollAt) return;
    lastPoll.current = pollAt;
    const counts = stateCounts(rows);
    if (rows.length > 0) {
      analytics?.track({
        name: "background_activity_rows",
        category: "user_action",
        component: "BackgroundActivity",
        labels: stateCountLabels(counts),
      });
    }
    alertRef.current ??= createEmptyEvidenceAlert({
      notify: () => notifyRef.current?.(),
      initial: loadAlertState(),
      onChange: saveAlertState,
    });
    alertRef.current.observe(counts, Date.now());
  }, [rows, pollAt, enabled, analytics]);
}
