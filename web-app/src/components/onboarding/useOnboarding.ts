import { useState, useEffect, useCallback } from "react";

const ONBOARDED_KEY = "stapler-squad:onboarded";

export function useOnboarding() {
  const [showOnboarding, setShow] = useState(false);

  useEffect(() => {
    try {
      if (!localStorage.getItem(ONBOARDED_KEY)) {
        setTimeout(() => setShow(true), 800);
      }
    } catch {
      // ignore storage errors (private browsing mode, etc.)
    }
  }, []);

  const setOnboardingComplete = useCallback(() => {
    try {
      localStorage.setItem(ONBOARDED_KEY, "true");
    } catch {
      // ignore storage errors
    }
    setShow(false);
  }, []);

  const resetOnboarding = useCallback(() => {
    try {
      localStorage.removeItem(ONBOARDED_KEY);
    } catch {
      // ignore storage errors
    }
    setShow(true);
  }, []);

  return { showOnboarding, setOnboardingComplete, resetOnboarding };
}
