"use client";

import { createContext, useContext, ReactNode, useCallback, useMemo } from "react";
import { useGetApprovalsQuery, useResolveApprovalMutation } from "@/lib/api/approvalsApi";
import type { PlainApproval } from "@/lib/api/approvalsApi";
import { toErrorOrNull } from "@/lib/utils/rtkQueryError";

export interface ApprovalsContextValue {
  approvals: PlainApproval[];
  pendingCount: number;
  loading: boolean;
  error: Error | null;
  approve: (approvalId: string) => Promise<void>;
  deny: (approvalId: string, message?: string) => Promise<void>;
  refresh: () => Promise<void>;
}

const ApprovalsContext = createContext<ApprovalsContextValue | null>(null);

/** Fallback returned by useApprovalsContext when used outside ApprovalsProvider. */
const noopAsync = async () => {};

const FALLBACK_CONTEXT: ApprovalsContextValue = {
  approvals: [],
  pendingCount: 0,
  loading: false,
  error: null,
  approve: noopAsync,
  deny: noopAsync,
  refresh: noopAsync,
};

export function ApprovalsProvider({ children }: { children: ReactNode }) {
  // RTK Query with 5s polling — single authoritative source for the entire app
  const { data, isLoading, error: queryError, refetch } = useGetApprovalsQuery(undefined, {
    pollingInterval: 5000,
  });

  const [resolveApproval] = useResolveApprovalMutation();

  const approve = useCallback(async (approvalId: string) => {
    await resolveApproval({ approvalId, decision: "allow" });
  }, [resolveApproval]);

  const deny = useCallback(async (approvalId: string, message?: string) => {
    await resolveApproval({ approvalId, decision: "deny", message });
  }, [resolveApproval]);

  const refresh = useCallback(async () => {
    await refetch();
  }, [refetch]);

  const approvals = data?.approvals ?? [];
  const error = toErrorOrNull(queryError);
  const pendingCount = approvals.length;

  const value = useMemo<ApprovalsContextValue>(
    () => ({ approvals, pendingCount, loading: isLoading, error, approve, deny, refresh }),
    [approvals, pendingCount, isLoading, error, approve, deny, refresh]
  );

  return (
    <ApprovalsContext.Provider value={value}>
      {children}
    </ApprovalsContext.Provider>
  );
}

/**
 * Returns the approvals context value.
 * Safe to call outside ApprovalsProvider — returns a no-op fallback instead of throwing.
 */
export function useApprovalsContext(): ApprovalsContextValue {
  const ctx = useContext(ApprovalsContext);
  return ctx ?? FALLBACK_CONTEXT;
}
