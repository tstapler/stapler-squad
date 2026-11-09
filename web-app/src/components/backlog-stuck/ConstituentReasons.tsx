"use client";

import React, { useState } from "react";
import type { StuckReason } from "@/gen/session/v1/backlog_pb";
import { useAnalytics } from "@/lib/analytics";
import { getStuckReasonLabel, hasRemediationAction } from "./stuckReason";

interface ConstituentReasonsProps {
  itemId: string;
  reasons: StuckReason[];
  onTriggerRemediationNow?: (itemId: string, reason: StuckReason) => Promise<void>;
}

/**
 * Lists the open reasons behind a MULTIPLE_REASONS row. multiple_reasons has no
 * remediation of its own, so each reason with a wired action gets its own Retry.
 */
export function ConstituentReasons({ itemId, reasons, onTriggerRemediationNow }: ConstituentReasonsProps) {
  const { track } = useAnalytics();
  const [pending, setPending] = useState<StuckReason | null>(null);
  const [errors, setErrors] = useState<Partial<Record<StuckReason, string>>>({});

  if (reasons.length === 0) return null;

  async function retry(reason: StuckReason) {
    if (!onTriggerRemediationNow || pending !== null) return;
    track({ name: "stuck_constituent_retry", category: "user_action", component: "ConstituentReasons" });
    setPending(reason);
    setErrors((prev) => ({ ...prev, [reason]: undefined }));
    try {
      await onTriggerRemediationNow(itemId, reason);
    } catch (err) {
      setErrors((prev) => ({ ...prev, [reason]: err instanceof Error ? err.message : "Retry failed" }));
    } finally {
      setPending(null);
    }
  }

  return (
    <ul data-testid="stuck-constituent-reasons">
      {reasons.map((reason) => (
        <li key={reason} data-testid={`stuck-constituent-reason-${reason}`}>
          {getStuckReasonLabel(reason)}
          {onTriggerRemediationNow && hasRemediationAction(reason) && (
            // Tracked inside retry(), after its pending guard.
            // analytics-exempt
            <button
              type="button"
              data-testid={`stuck-constituent-retry-${reason}`}
              disabled={pending !== null}
              onClick={() => void retry(reason)}
            >
              {pending === reason ? "Retrying…" : "Retry"}
            </button>
          )}
          {errors[reason] && <span role="alert"> {errors[reason]}</span>}
        </li>
      ))}
    </ul>
  );
}
