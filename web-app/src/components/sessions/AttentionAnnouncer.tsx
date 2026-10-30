"use client";

import { useEffect, useRef, useState } from "react";
import { subscribeAttentionAnnouncements } from "@/lib/attention/announceAttention";
import { AnnouncementQueue, COALESCE_WINDOW_MS } from "@/lib/attention/announcementQueue";
import { visuallyHidden } from "@/styles/a11y.css";

const SPEAK_GAP_MS = 2000;

/**
 * The one polite live region for the attention list. Mount once in the app shell, outside any
 * subtree that becomes inert or aria-hidden, so it still speaks while a dialog is open.
 */
export function AttentionAnnouncer() {
  const [spoken, setSpoken] = useState<{ text: string; seq: number } | null>(null);
  const queue = useRef(new AnnouncementQueue());
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const seq = useRef(0);

  useEffect(() => {
    const flush = () => {
      timer.current = null;
      const text = queue.current.take(Date.now());
      if (text === null) return;
      seq.current += 1;
      setSpoken({ text, seq: seq.current });
      if (queue.current.size() > 0) timer.current = setTimeout(flush, SPEAK_GAP_MS);
    };
    const unsubscribe = subscribeAttentionAnnouncements((a) => {
      queue.current.enqueue(a.kind, a.format, Date.now(), a.count);
      // The first event opens a burst window; later ones inside it coalesce before anything is spoken.
      if (timer.current === null) timer.current = setTimeout(flush, COALESCE_WINDOW_MS);
    });
    return () => {
      unsubscribe();
      if (timer.current !== null) clearTimeout(timer.current);
      timer.current = null;
    };
  }, []);

  return (
    <div
      role="status"
      aria-live="polite"
      aria-atomic="true"
      className={visuallyHidden}
      data-testid="attention-announcer"
    >
      {/* Re-keyed so an identical message still reads as a change. */}
      {spoken && <span key={spoken.seq}>{spoken.text}</span>}
    </div>
  );
}
