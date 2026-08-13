"use client";

import { useEffect, useState } from "react";
import { srOnly } from "./LiveRegion.css";

interface LiveRegionProps {
  message: string;
  politeness?: "polite" | "assertive";
}

export function LiveRegion({ message, politeness = "polite" }: LiveRegionProps) {
  const [currentMessage, setCurrentMessage] = useState(message);

  useEffect(() => {
    if (message) {
      setCurrentMessage(message);
    }
  }, [message]);

  return (
    <div
      role="status"
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
