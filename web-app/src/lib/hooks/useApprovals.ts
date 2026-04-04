"use client";

import { useEffect, useCallback, useRef, useMemo } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { PendingApprovalProto } from "@/gen/session/v1/types_pb";
import {
  ListPendingApprovalsRequest,
  ListPendingApprovalsRequestSchema,
  ResolveApprovalRequest,
  ResolveApprovalRequestSchema,
} from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { getApiBaseUrl } from "@/lib/config";
import { useAppDispatch, useAppSelector } from "@/lib/store";
import {
  setApprovals,
  setLoading,
  setError,
  removeApproval,
  selectApprovals,
  selectApprovalsLoading,
  selectApprovalsError,
} from "@/lib/store/approvalsSlice";

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

  const dispatch = useAppDispatch();
  const approvals = useAppSelector(selectApprovals);
  const loading = useAppSelector(selectApprovalsLoading);
  const errorStr = useAppSelector(selectApprovalsError);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  // Initialize ConnectRPC client
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
    });
    clientRef.current = createClient(SessionService, transport);
  }, []);

  // Fetch pending approvals
  const fetchApprovals = useCallback(async () => {
    if (!clientRef.current) return;

    dispatch(setLoading(true));
    dispatch(setError(null));

    try {
      const request = create(ListPendingApprovalsRequestSchema, {});
      if (sessionId !== undefined) {
        request.sessionId = sessionId;
      }

      const response = await clientRef.current.listPendingApprovals(request);
      dispatch(setApprovals(response.approvals ?? []));
      dispatch(setError(null));
    } catch (err) {
      const error =
        err instanceof Error
          ? err
          : new Error("Failed to fetch pending approvals");
      dispatch(setError(error.message));
      console.error("Failed to fetch pending approvals:", error);
    } finally {
      dispatch(setLoading(false));
    }
  }, [sessionId, dispatch]);

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
      dispatch(removeApproval(approvalId));

      try {
        const request = create(ResolveApprovalRequestSchema, {
          approvalId,
          decision: "allow",
        });
        await clientRef.current.resolveApproval(request);
      } catch (err) {
        console.error("Failed to approve:", err);
        dispatch(
          setError(
            err instanceof Error ? err.message : "Failed to approve request"
          )
        );
        // Rollback - refetch to get correct state
        await refresh();
      }
    },
    [refresh, dispatch]
  );

  // Deny a pending approval
  const deny = useCallback(
    async (approvalId: string, message?: string) => {
      if (!clientRef.current) return;

      // Optimistic update - remove from list immediately
      dispatch(removeApproval(approvalId));

      try {
        const request = create(ResolveApprovalRequestSchema, {
          approvalId,
          decision: "deny",
          message,
        });
        await clientRef.current.resolveApproval(request);
      } catch (err) {
        console.error("Failed to deny:", err);
        dispatch(
          setError(
            err instanceof Error ? err.message : "Failed to deny request"
          )
        );
        // Rollback - refetch to get correct state
        await refresh();
      }
    },
    [refresh, dispatch]
  );

  // Convert error string back to Error object for backward compatibility.
  // Memoised so the Error identity stays stable across renders.
  const error = useMemo(() => (errorStr ? new Error(errorStr) : null), [errorStr]);

  return {
    approvals,
    loading,
    error,
    approve,
    deny,
    refresh,
  };
}
