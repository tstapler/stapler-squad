"use client";

import { useCallback, useContext } from "react";
import { AnalyticsContext } from "@/lib/contexts/AnalyticsContext";
import { ActionKind } from "@/gen/session/v1/notification_action_pb";

/** The server counts this name into inline_action_undo_total{kind}. */
export const INLINE_ACTION_UNDO_EVENT = "inline_action_undo";

/** Undo cancels during the commit delay, so only the client can report it. Inert without a provider. */
export function useInlineActionTelemetry() {
  const analytics = useContext(AnalyticsContext);
  const trackUndo = useCallback(
    (kind: ActionKind) => {
      analytics?.track({
        name: INLINE_ACTION_UNDO_EVENT,
        category: "user_action",
        component: "ActionRow",
        labels: { kind: (ActionKind[kind] ?? "unknown").toLowerCase() },
      });
    },
    [analytics],
  );
  return { trackUndo };
}
