"use client";

import { createContext, useContext, ReactNode } from "react";
import { useApprovals } from "@/lib/hooks/useApprovals";

type ApprovalsContextValue = ReturnType<typeof useApprovals>;
const ApprovalsContext = createContext<ApprovalsContextValue | null>(null);

export function ApprovalsProvider({ children }: { children: ReactNode }) {
  const value = useApprovals({ pollInterval: 30000 });
  return <ApprovalsContext.Provider value={value}>{children}</ApprovalsContext.Provider>;
}

export function useApprovalsContext() {
  const ctx = useContext(ApprovalsContext);
  if (!ctx) throw new Error("useApprovalsContext must be used within ApprovalsProvider");
  return ctx;
}
