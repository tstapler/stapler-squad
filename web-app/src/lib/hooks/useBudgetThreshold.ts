"use client";

import { useState, useEffect, useCallback } from "react";

const STORAGE_KEY = "insights_budget_threshold_usd";

export interface UseBudgetThresholdReturn {
  threshold: number | null;
  setThreshold: (v: number | null) => void;
  isHydrated: boolean;
}

/** Persists a budget threshold in localStorage with SSR hydration guard. */
export function useBudgetThreshold(): UseBudgetThresholdReturn {
  const [threshold, setThresholdState] = useState<number | null>(null);
  const [isHydrated, setIsHydrated] = useState(false);

  useEffect(() => {
    try {
      const stored = window.localStorage.getItem(STORAGE_KEY);
      if (stored !== null) {
        const parsed = parseFloat(stored);
        if (!isNaN(parsed)) setThresholdState(parsed);
      }
    } catch {
      // ignore
    }
    setIsHydrated(true);
  }, []);

  const setThreshold = useCallback((v: number | null) => {
    setThresholdState(v);
    try {
      if (v === null) {
        window.localStorage.removeItem(STORAGE_KEY);
      } else {
        window.localStorage.setItem(STORAGE_KEY, String(v));
      }
    } catch {
      // ignore
    }
  }, []);

  return { threshold, setThreshold, isHydrated };
}
