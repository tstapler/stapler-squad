"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";
import { PendingApprovalProto } from "@/gen/session/v1/types_pb";
import {
  ListPendingApprovalsRequest,
  ResolveApprovalRequest,
} from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";

interface UseApprovalsOptions {
  sessionId?: string;
  pollInterval?: number; // in milliseconds, default 30000
  /**
   * Increment this counter externally to trigger an immediate refresh.
   * Use when the parent receives an APPROVAL_NEEDED notification so the
   * panel updates without waiting for the next poll cycle.
   */
  notificationTrigger?: number;
}

interface UseApprovalsReturn {
  approvals: PendingApprovalProto[];
  loading: boolean;
  error: Error | null;
  approve: (approvalId: string) => Promise<void>;
  deny: (approvalId: string, message?: string) => Promise<void>;
  refresh: () => Promise<void>;
}

/**
 * React hook for managing pending tool-use approval requests.
 *
 * Polls `listPendingApprovals` every 3 seconds and exposes approve/deny actions
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
  const { sessionId, pollInterval = 30000, notificationTrigger } = options;

  const [approvals, setApprovals] = useState<PendingApprovalProto[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const clientRef = useRef<ReturnType<typeof createPromiseClient<typeof SessionService>> | null>(null);

  // Initialize ConnectRPC client
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
    });
    clientRef.current = createPromiseClient(SessionService, transport);
  }, []);

  // Fetch pending approvals
  const fetchApprovals = useCallback(async () => {
    if (!clientRef.current) return;

    setLoading(true);
    setError(null);

    try {
      const request = new ListPendingApprovalsRequest();
      if (sessionId !== undefined) {
        request.sessionId = sessionId;
      }

      const response = await clientRef.current.listPendingApprovals(request);
      setApprovals(response.approvals ?? []);
      setError(null);
    } catch (err) {
      const error =
        err instanceof Error
          ? err
          : new Error("Failed to fetch pending approvals");
      setError(error);
      console.error("Failed to fetch pending approvals:", error);
    } finally {
      setLoading(false);
    }
  }, [sessionId]);

  // Refresh alias
  const refresh = useCallback(async () => {
    await fetchApprovals();
  }, [fetchApprovals]);

  // Setup polling
  useEffect(() => {
    // Initial fetch
    refresh();

    const interval = setInterval(() => {
      refresh();
    }, pollInterval);

    return () => {
      clearInterval(interval);
    };
  }, [pollInterval, refresh]);

  // Immediate refresh when a notification arrives (via notificationTrigger counter)
  useEffect(() => {
    if (notificationTrigger === undefined || notificationTrigger === 0) return;
    refresh();
  }, [notificationTrigger, refresh]);

  // Approve a pending approval
  const approve = useCallback(
    async (approvalId: string) => {
      if (!clientRef.current) return;

      // Optimistic update - remove from list immediately
      setApprovals((prev) => prev.filter((a) => a.id !== approvalId));

      try {
        const request = new ResolveApprovalRequest({
          approvalId,
          decision: "allow",
        });
        await clientRef.current.resolveApproval(request);
      } catch (err) {
        console.error("Failed to approve:", err);
        setError(
          err instanceof Error ? err : new Error("Failed to approve request")
        );
        // Rollback - refetch to get correct state
        await refresh();
      }
    },
    [refresh]
  );

  // Deny a pending approval
  const deny = useCallback(
    async (approvalId: string, message?: string) => {
      if (!clientRef.current) return;

      // Optimistic update - remove from list immediately
      setApprovals((prev) => prev.filter((a) => a.id !== approvalId));

      try {
        const request = new ResolveApprovalRequest({
          approvalId,
          decision: "deny",
          message,
        });
        await clientRef.current.resolveApproval(request);
      } catch (err) {
        console.error("Failed to deny:", err);
        setError(
          err instanceof Error ? err : new Error("Failed to deny request")
        );
        // Rollback - refetch to get correct state
        await refresh();
      }
    },
    [refresh]
  );

  return {
    approvals,
    loading,
    error,
    approve,
    deny,
    refresh,
  };
}
