"use client";

import { useEffect, useState } from "react";
import { srOnly } from "./LiveRegion.css";

interface LiveRegionProps {
  message: string;
  politeness?: "polite" | "assertive";
  /** ARIA role for the live region. Defaults to "status" (existing behavior). Pass "alert" for assertive, interrupt-worthy announcements (Task 2.3.1). */
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
