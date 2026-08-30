"use client";

import { createContext, useContext } from "react";
import type { SessionViewMode } from "@/lib/hooks/useSessionViewMode";

export interface SessionViewModeValue {
  viewMode: SessionViewMode;
  setViewMode: (mode: SessionViewMode) => void;
}

const SessionViewModeContext = createContext<SessionViewModeValue | null>(null);

export function useSessionViewModeContext(): SessionViewModeValue {
  const ctx = useContext(SessionViewModeContext);
  if (!ctx) throw new Error("useSessionViewModeContext must be inside SessionViewModeProvider");
  return ctx;
}

export const SessionViewModeProvider = SessionViewModeContext.Provider;
