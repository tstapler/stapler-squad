import { useState, useEffect, useCallback } from "react";

export const BACKLOG_ONBOARDED_KEY = "stapler-squad:backlog-onboarded";

/** First-visit walkthrough state for the backlog page. Mirrors useOnboarding.ts. */
export function useBacklogTour() {
  const [showTour, setShow] = useState(false);

  useEffect(() => {
    let timerId: ReturnType<typeof setTimeout>;
    try {
      if (!localStorage.getItem(BACKLOG_ONBOARDED_KEY)) {
        timerId = setTimeout(() => setShow(true), 500);
      }
    } catch {
      // ignore storage errors (private browsing mode, etc.)
    }
    return () => clearTimeout(timerId);
  }, []);

  const setTourComplete = useCallback(() => {
    try {
      localStorage.setItem(BACKLOG_ONBOARDED_KEY, "true");
    } catch {
      // ignore storage errors
    }
    setShow(false);
  }, []);

  // Hides the modal without marking the tour as permanently seen — used when
  // the user explicitly unchecks "Don't show this again" and clicks "Got it".
  const hideTour = useCallback(() => {
    setShow(false);
  }, []);

  const resetTour = useCallback(() => {
    try {
      localStorage.removeItem(BACKLOG_ONBOARDED_KEY);
    } catch {
      // ignore storage errors
    }
    setShow(true);
  }, []);

  return { showTour, setTourComplete, hideTour, resetTour };
}
