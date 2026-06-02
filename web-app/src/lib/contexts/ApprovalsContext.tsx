"use client";

import { createContext, useContext, ReactNode, useCallback, useMemo, useState, useRef } from "react";
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
  clearForSession: (sessionId: string) => void;
  clearedSessions: ReadonlySet<string>;
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
  clearForSession: () => {},
  clearedSessions: new Set(),
};

export function ApprovalsProvider({ children }: { children: ReactNode }) {
  // RTK Query with 5s polling — single authoritative source for the entire app
  const { data, isLoading, error: queryError, refetch } = useGetApprovalsQuery(undefined, {
    pollingInterval: 5000,
  });

  const [resolveApproval] = useResolveApprovalMutation();
  const [clearedSessions, setClearedSessions] = useState<Set<string>>(new Set());
  // In-flight count per session prevents premature removal when Enter is pressed twice rapidly
  const clearCountRef = useRef<Record<string, number>>({});

  const approve = useCallback(async (approvalId: string) => {
    await resolveApproval({ approvalId, decision: "allow" });
  }, [resolveApproval]);

  const deny = useCallback(async (approvalId: string, message?: string) => {
    await resolveApproval({ approvalId, decision: "deny", message });
  }, [resolveApproval]);

  const refresh = useCallback(async () => {
    await refetch();
  }, [refetch]);

  const clearForSession = useCallback((sessionId: string) => {
    clearCountRef.current[sessionId] = (clearCountRef.current[sessionId] ?? 0) + 1;
    setClearedSessions(prev => new Set(prev).add(sessionId));
    void (async () => {
      try {
        await refetch();
      } catch (err) {
        console.error("[ApprovalsContext] refetch failed during optimistic clear:", err);
      } finally {
        clearCountRef.current[sessionId]--;
        if ((clearCountRef.current[sessionId] ?? 0) <= 0) {
          delete clearCountRef.current[sessionId];
          setClearedSessions(prev => {
            const next = new Set(prev);
            next.delete(sessionId);
            return next;
          });
        }
      }
    })();
  }, [refetch]);

  const filteredApprovals = useMemo(
    () => (data?.approvals ?? []).filter(a => !clearedSessions.has(a.sessionId)),
    [data, clearedSessions]
  );

  const error = toErrorOrNull(queryError);
  const pendingCount = filteredApprovals.length;

  const value = useMemo<ApprovalsContextValue>(
    () => ({ approvals: filteredApprovals, pendingCount, loading: isLoading, error, approve, deny, refresh, clearForSession, clearedSessions }),
    [filteredApprovals, pendingCount, isLoading, error, approve, deny, refresh, clearForSession, clearedSessions]
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
