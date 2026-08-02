"use client";

import { useEffect, useState } from "react";
import { srOnly } from "./LiveRegion.css";

interface LiveRegionProps {
  message: string;
  politeness?: "polite" | "assertive";
  /** Task 4.2.2.1 — defaults to "status" (backward compatible: no existing
   *  real consumer relied on a specific role before this option existed). */
  role?: "status" | "alert";
}

export function LiveRegion({ message, politeness = "polite", role = "status" }: LiveRegionProps) {
  const [currentMessage, setCurrentMessage] = useState(message);

  useEffect(() => {
    if (message) {
      setCurrentMessage(message);
    }
  }, [message]);

  return (
    <div
      role={role}
      aria-live={politeness}
      aria-atomic="true"
      className={srOnly}
    >
      {currentMessage}
    </div>
  );
}

// Hook to use live region announcements
export function useLiveRegion() {
  const [message, setMessage] = useState("");

  const announce = (newMessage: string) => {
    setMessage(newMessage);
    // Clear message after announcement
    setTimeout(() => setMessage(""), 1000);
  };

  return { message, announce };
}
