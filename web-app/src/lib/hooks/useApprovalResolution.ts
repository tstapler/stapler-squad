import { useCallback, useEffect, useRef, useState } from "react";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { SessionService } from "@/gen/session/v1/session_pb";
import { ResolveApprovalRequestSchema } from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { NotificationHistoryItem } from "@/lib/types/notification";

export type ApprovalDecisionState = "allow" | "deny" | "expired";

export interface UseApprovalResolutionOptions {
  notificationHistory: NotificationHistoryItem[];
  /** Marks the given notification id(s) as read/acknowledged and closes any active toast. */
  acknowledgeNotification: (notificationIds: string | string[]) => void;
}

export interface UseApprovalResolutionResult {
  /** "allow"/"deny" = user resolved it; "expired" = already resolved or timed out. */
  resolvedApprovals: Record<string, ApprovalDecisionState>;
  /** Approval IDs with an in-flight RPC — used to disable buttons while waiting. */
  pendingApprovals: Record<string, boolean>;
  /** Approvals blocked by the CI-red guard (AC5), keyed by approval ID, valued by the block message. */
  blockedApprovals: Record<string, string>;
  resolveApproval: (
    approvalId: string,
    decision: "allow" | "deny",
    notificationIds: string | string[],
    overrideCiBlock?: boolean
  ) => Promise<void>;
}

/**
 * Shared approval-resolution state/logic for the notification history views
 * (NotificationsPage and NotificationPanel): seeds resolved state from persisted
 * notification metadata, tracks in-flight/blocked approvals, and resolves an
 * approval via RPC.
 */
export function useApprovalResolution({
  notificationHistory,
  acknowledgeNotification,
}: UseApprovalResolutionOptions): UseApprovalResolutionResult {
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const getClient = useCallback(() => {
    if (!clientRef.current) {
      clientRef.current = createClient(SessionService, getConnectTransport());
    }
    return clientRef.current;
  }, []);

  const [resolvedApprovals, setResolvedApprovals] = useState<Record<string, ApprovalDecisionState>>({});
  const [pendingApprovals, setPendingApprovals] = useState<Record<string, boolean>>({});
  const [blockedApprovals, setBlockedApprovals] = useState<Record<string, string>>({});

  // Seed resolvedApprovals from persisted metadata when history loads or updates.
  // The server stamps "approval_decision" on the notification record when an approval
  // is resolved, so this survives page refreshes.
  // "timeout" and "canceled" are server-side outcomes where the approval was not handled
  // via the UI; we map them to "expired" for display (same "Expired" badge).
  useEffect(() => {
    const seeded: Record<string, ApprovalDecisionState> = {};
    for (const n of notificationHistory) {
      const decision = n.metadata?.["approval_decision"];
      const approvalId = n.metadata?.["approval_id"];
      if (!approvalId || !decision) continue;
      if (decision === "allow" || decision === "deny") {
        seeded[approvalId] = decision;
      } else if (decision === "timeout" || decision === "canceled") {
        seeded[approvalId] = "expired";
      }
    }
    if (Object.keys(seeded).length > 0) {
      setResolvedApprovals(prev => ({ ...seeded, ...prev }));
    }
  }, [notificationHistory]);

  const resolveApproval = useCallback(async (approvalId: string, decision: "allow" | "deny", notificationIds: string | string[], overrideCiBlock?: boolean) => {
    setPendingApprovals(prev => ({ ...prev, [approvalId]: true }));
    try {
      // One-shot mutation triggered by an explicit user action (Approve/Deny
      // click), not tied to a mount/effect.
      // abort-signal-exempt
      await getClient().resolveApproval(create(ResolveApprovalRequestSchema, { approvalId, decision, overrideCiBlock }));
      setResolvedApprovals(prev => ({ ...prev, [approvalId]: decision }));
      setBlockedApprovals(prev => { const next = { ...prev }; delete next[approvalId]; return next; });
      // Single call: marks as read in history AND closes the active toast
      acknowledgeNotification(notificationIds);
    } catch (err) {
      // AC5: CI-red block — show an inline explanation next to Approve/Deny instead of
      // collapsing into the generic "expired" state (not a silent no-op).
      if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
        setBlockedApprovals(prev => ({ ...prev, [approvalId]: err.rawMessage || err.message }));
        return;
      }
      console.error("Failed to resolve approval:", err);
      // Approval already timed out or was resolved elsewhere — mark as expired.
      setResolvedApprovals(prev => ({ ...prev, [approvalId]: "expired" }));
    } finally {
      setPendingApprovals(prev => { const next = { ...prev }; delete next[approvalId]; return next; });
    }
  }, [getClient, acknowledgeNotification]);

  return { resolvedApprovals, pendingApprovals, blockedApprovals, resolveApproval };
}
