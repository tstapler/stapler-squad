"use client";

import { useMemo } from "react";
import { useApprovalsContext } from "@/lib/contexts/ApprovalsContext";
import type { PlainApproval } from "@/lib/api/approvalsApi";
import type { AsyncResult } from "@/lib/types/asyncResult";

interface UseApprovalsOptions {
  sessionId?: string;
  /**
   * @deprecated Polling is now controlled centrally by ApprovalsProvider.
   * This option is accepted for backwards-compatibility but has no effect.
   */
  pollInterval?: number;
  /**
   * @deprecated No longer has any effect; kept for backwards-compatibility.
   */
  notificationTrigger?: number;
}

export interface UseApprovalsReturn extends AsyncResult {
  approvals: PlainApproval[];
  approve: (approvalId: string) => Promise<void>;
  deny: (approvalId: string, message?: string) => Promise<void>;
  refresh: () => Promise<void>;
}

/**
 * React hook for managing pending tool-use approval requests.
 *
 * Delegates to ApprovalsContext (the single RTK Query polling singleton).
 * Optionally filters the approval list by sessionId.
 *
 * @example
 * ```tsx
 * const { approvals, approve, deny } = useApprovals({ sessionId: "abc" });
 *
 * // Approve a tool-use request
 * await approve("approval-123");
 *
 * // Deny with a message
 * await deny("approval-123", "This command is not safe");
 * ```
 */
export function useApprovals(
  options: UseApprovalsOptions = {}
): UseApprovalsReturn {
  const { sessionId } = options;
  const { approvals: allApprovals, loading, error, approve, deny, refresh } =
    useApprovalsContext();

  const approvals = useMemo(
    () =>
      sessionId
        ? allApprovals.filter((a) => a.sessionId === sessionId)
        : allApprovals,
    [allApprovals, sessionId]
  );

  return { approvals, loading, error, approve, deny, refresh };
}
