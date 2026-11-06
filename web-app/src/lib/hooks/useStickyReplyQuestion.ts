"use client";

import { useEffect, useState } from "react";

/**
 * Keeps the question a Reply card is showing after its notification turns read.
 * Sending a reply marks the notification read, which would otherwise unmount the
 * card and its receipt in the same render (RP-4: the receipt stays until the
 * question's state changes or the view closes). A different unread question
 * replaces it, and a different session clears it.
 */
export function useStickyReplyQuestion<T extends { id: string }>(unread: T | undefined, sessionId: string): T | undefined {
  const [held, setHeld] = useState<{ sessionId: string; question: T | undefined }>({ sessionId, question: unread });

  useEffect(() => {
    setHeld((prev) => {
      if (prev.sessionId !== sessionId) return { sessionId, question: unread };
      if (unread && unread.id !== prev.question?.id) return { sessionId, question: unread };
      return prev;
    });
  }, [unread, sessionId]);

  if (held.sessionId !== sessionId) return unread;
  return unread ?? held.question;
}
