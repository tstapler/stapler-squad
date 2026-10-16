import { useCallback, useState } from "react";
import type { StuckReason } from "@/gen/session/v1/backlog_pb";
import { getStuckReasonLabel } from "./stuckReason";

/**
 * "Reset parked" state shared by the global "Reset all parked" button and
 * each per-reason-group "Reset parked (N)" button. Extracted from
 * StuckItemsSection.tsx — same behavior, isolated as its own concern. Reason
 * omitted resets across every reason (unchanged existing behavior), reason
 * set scopes the reset (and the resulting message) to just that bucket.
 */
export function useBulkResetParked(bulkResetParkedRemediation: (reason?: StuckReason) => Promise<number>) {
  const [bulkResetState, setBulkResetState] = useState<"idle" | "pending" | "error">("idle");
  const [bulkResetMessage, setBulkResetMessage] = useState<string | null>(null);
  // Which single reason's "Reset parked (N)" button is mid-flight — kept
  // separate from bulkResetState (the global "Reset all parked" button's own
  // pending flag) so each button can show its own label/spinner, but both are
  // combined below into anyResetPending so the two families still can't fire
  // overlapping resets that would clobber the single shared bulkResetMessage.
  const [resettingReason, setResettingReason] = useState<StuckReason | null>(null);

  // True while either the global or a per-reason reset is in flight — gates
  // both button families so an operator can't fire two overlapping
  // BulkResetStuckRemediation calls whose resulting toasts would clobber the
  // single shared bulkResetMessage state.
  const anyResetPending = bulkResetState === "pending" || resettingReason !== null;

  const handleBulkResetParked = useCallback(
    async (reason?: StuckReason) => {
      if (anyResetPending) {
        return;
      }
      if (reason !== undefined) {
        setResettingReason(reason);
      } else {
        setBulkResetState("pending");
      }
      setBulkResetMessage(null);
      try {
        const n = await bulkResetParkedRemediation(reason);
        setBulkResetState("idle");
        const scope = reason !== undefined ? ` in ${getStuckReasonLabel(reason)}` : "";
        setBulkResetMessage(
          n > 0 ? `Reset ${n} parked item${n !== 1 ? "s" : ""}${scope}.` : "No parked items to reset."
        );
      } catch (err) {
        setBulkResetState("error");
        setBulkResetMessage(err instanceof Error ? err.message : "Bulk reset failed");
      } finally {
        setResettingReason(null);
      }
    },
    [anyResetPending, bulkResetParkedRemediation]
  );

  return { bulkResetState, bulkResetMessage, resettingReason, anyResetPending, handleBulkResetParked };
}
