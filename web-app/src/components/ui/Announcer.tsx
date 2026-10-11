"use client";

import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { AnnouncerContext, AnnouncerEngine, type AnnounceApi } from "@/lib/hooks/useAnnounce";
import { srOnly } from "./LiveRegion.css";

/**
 * The one owner of the notification system's live regions: an always-mounted
 * polite `role="status"` and an always-mounted assertive `role="alert"`. They
 * carry only the role (its implicit aria-live), never an extra aria-live, so no
 * reader double-announces. Surfaces call `useAnnounce()`; the API is documented
 * in lib/hooks/useAnnounce.ts.
 *
 * Reuses LiveRegion's visually-hidden style rather than the LiveRegion component:
 * that component retains the last message and sets aria-live beside the role,
 * which would defeat clear-then-repeat announcements.
 */
export function AnnouncerProvider({ children }: { children?: ReactNode }) {
  const [polite, setPolite] = useState("");
  const [assertive, setAssertive] = useState("");
  const engineRef = useRef<AnnouncerEngine | null>(null);
  if (engineRef.current === null) {
    engineRef.current = new AnnouncerEngine({ setPolite, setAssertive });
  }

  useEffect(() => {
    const engine = engineRef.current;
    engine?.resume();
    return () => engine?.dispose();
  }, []);

  const api = useMemo<AnnounceApi>(
    () => ({
      announce: (message, politeness, key) => engineRef.current?.announce(message, politeness, key),
      announceArrival: (arrival) => engineRef.current?.announceArrival(arrival),
    }),
    [],
  );

  return (
    <AnnouncerContext.Provider value={api}>
      {children}
      <div role="status" aria-atomic="true" data-testid="announcer-polite" className={srOnly}>
        {polite}
      </div>
      <div role="alert" aria-atomic="true" data-testid="announcer-assertive" className={srOnly}>
        {assertive}
      </div>
    </AnnouncerContext.Provider>
  );
}
