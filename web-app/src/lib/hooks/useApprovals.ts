"use client";

import { useCallback, useMemo } from "react";
import { useGetApprovalsQuery, useResolveApprovalMutation } from "@/lib/api/approvalsApi";
import type { PlainApproval } from "@/lib/api/approvalsApi";

interface UseApprovalsOptions {
  sessionId?: string;
  pollInterval?: number; // in milliseconds, default 5000
  /**
   * Increment this counter externally to trigger an immediate refresh.
   * Use when the parent receives an APPROVAL_NEEDED notification so the
   * panel updates without waiting for the next poll cycle.
   */
  notificationTrigger?: number;
}

interface UseApprovalsReturn {
  approvals: PlainApproval[];
  loading: boolean;
  error: Error | null;
  approve: (approvalId: string) => Promise<void>;
  deny: (approvalId: string, message?: string) => Promise<void>;
  refresh: () => Promise<void>;
}

/**
 * React hook for managing pending tool-use approval requests.
 *
 * Polls `listPendingApprovals` (via RTK Query) and exposes approve/deny actions
 * that call `resolveApproval` on the ConnectRPC SessionService.
 *
 * Pass `notificationTrigger` (increment it on APPROVAL_NEEDED events) to get
 * near-instant updates without opening an additional streaming connection.
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
  const { sessionId, pollInterval = 5000 } = options;

  const { data, isLoading, error: queryError, refetch } = useGetApprovalsQuery(undefined, {
    pollingInterval: pollInterval,
  });

  const [resolveApproval] = useResolveApprovalMutation();

  // Filter by sessionId if provided
  const approvals = useMemo(() => {
    const all = data?.approvals ?? [];
    return sessionId ? all.filter((a) => a.sessionId === sessionId) : all;
  }, [data?.approvals, sessionId]);

  const approve = useCallback(
    async (approvalId: string) => {
      await resolveApproval({ approvalId, decision: "allow" });
    },
    [resolveApproval]
  );

  const deny = useCallback(
    async (approvalId: string, message?: string) => {
      await resolveApproval({ approvalId, decision: "deny", message });
    },
    [resolveApproval]
  );

  const refresh = useCallback(async () => {
    await refetch();
  }, [refetch]);

  const error = useMemo(() => {
    if (!queryError) return null;
    const msg =
      typeof queryError === "object" && "error" in queryError
        ? String((queryError as { error: unknown }).error)
        : "Unknown error";
    return new Error(msg);
  }, [queryError]);

  return {
    approvals,
    loading: isLoading,
    error,
    approve,
    deny,
    refresh,
  };
}
