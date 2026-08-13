"use client";
// +feature: ui:memory-pressure-indicator

import { createContext, useContext, ReactNode } from "react";
import { useSessionServiceContext } from "@/lib/contexts/SessionServiceContext";

/** Default memory pressure threshold (mirrors backend HibernationConfig default). */
export const MEMORY_PRESSURE_THRESHOLD = 85;

interface SystemMemoryContextValue {
  /** System-wide memory usage percentage (0–100). Zero when unavailable. */
  systemMemoryPct: number;
  /** True when systemMemoryPct >= MEMORY_PRESSURE_THRESHOLD. */
  isUnderPressure: boolean;
}

const SystemMemoryContext = createContext<SystemMemoryContextValue>({
  systemMemoryPct: 0,
  isUnderPressure: false,
});

export function SystemMemoryProvider({ children }: { children: ReactNode }) {
  const { systemMemoryPct } = useSessionServiceContext();
  const isUnderPressure = systemMemoryPct >= MEMORY_PRESSURE_THRESHOLD;

  return (
    <SystemMemoryContext.Provider value={{ systemMemoryPct, isUnderPressure }}>
      {children}
    </SystemMemoryContext.Provider>
  );
}

export function useSystemMemory(): SystemMemoryContextValue {
  return useContext(SystemMemoryContext);
}
