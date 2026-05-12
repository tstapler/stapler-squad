"use client";

import { createContext, useContext, useRef, useMemo, useEffect, ReactNode } from "react";
import type { AnalyticsProvider } from "@/lib/analytics/types";

interface AnalyticsContextValue {
  provider: AnalyticsProvider;
  track: AnalyticsProvider["track"];
}

const AnalyticsContext = createContext<AnalyticsContextValue | null>(null);

export function useAnalytics(): AnalyticsContextValue {
  const context = useContext(AnalyticsContext);
  if (!context) {
    throw new Error("useAnalytics must be used within an AnalyticsContextProvider");
  }
  return context;
}

interface AnalyticsContextProviderProps {
  provider: AnalyticsProvider;
  children: ReactNode;
}

export function AnalyticsContextProvider({ provider, children }: AnalyticsContextProviderProps) {
  // Stable ref to the provider so identity doesn't change across parent re-renders
  const providerRef = useRef<AnalyticsProvider>(provider);
  providerRef.current = provider;

  useEffect(() => {
    const p = provider;
    providerRef.current = p;
    void p.initialize?.();
    return () => {
      void p.onClose?.();
    };
  }, [provider]);

  const contextValue = useMemo<AnalyticsContextValue>(
    () => ({
      provider: providerRef.current,
      track: (event) => providerRef.current.track(event),
    }),
    // track delegates through the ref so consumers don't re-render on provider swap
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []
  );

  return (
    <AnalyticsContext.Provider value={contextValue}>
      {children}
    </AnalyticsContext.Provider>
  );
}
