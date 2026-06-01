"use client";

import { ClaudeMessage } from "@/gen/session/v1/session_pb";
import { useEffect, useRef, useState } from "react";
import * as styles from "./HistoryCardPreview.css";

// Strips ANSI escape sequences (SGR + OSC hyperlinks) without an npm dependency.
function stripAnsi(str: string): string {
  return str
    .replace(/\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g, "") // OSC sequences (hyperlinks, etc.)
    .replace(/\x1b[@-Z\\-_]|\x1b\[[0-9;]*[A-Za-z]/g, ""); // CSI / single-char escapes
}

interface HistoryCardPreviewProps {
  entryId: string;
  isVisible: boolean;
  fetchMessages: (id: string) => Promise<ClaudeMessage[]>;
}

export function HistoryCardPreview({ entryId, isVisible, fetchMessages }: HistoryCardPreviewProps) {
  const [messages, setMessages] = useState<ClaudeMessage[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Cache fetched messages per entryId so re-expanding doesn't re-fetch.
  const cacheRef = useRef<Map<string, ClaudeMessage[]>>(new Map());

  useEffect(() => {
    if (!isVisible) return;
    const cached = cacheRef.current.get(entryId);
    if (cached) { setMessages(cached); return; }
    let cancelled = false;
    setLoading(true);
    setError(null);
    fetchMessages(entryId)
      .then((msgs) => {
        if (cancelled) return;
        cacheRef.current.set(entryId, msgs);
        setMessages(msgs);
      })
      .catch((err) => {
        if (!cancelled) setError(String(err));
      })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [entryId, isVisible, fetchMessages]);

  if (loading) {
    return (
      <div id={`preview-${entryId}`} className={styles.previewContainer}>
        <div className={styles.previewLoading}>Loading messages…</div>
      </div>
    );
  }
  if (error) {
    return (
      <div id={`preview-${entryId}`} className={styles.previewContainer}>
        <div className={styles.previewError}>Failed to load preview</div>
      </div>
    );
  }
  if (!messages || messages.length === 0) {
    return (
      <div id={`preview-${entryId}`} className={styles.previewContainer}>
        <div className={styles.previewEmpty}>No messages available</div>
      </div>
    );
  }

  return (
    <div id={`preview-${entryId}`} className={styles.previewContainer}>
      {messages.map((msg, idx) => (
        <div
          key={idx}
          className={`${styles.previewMessage} ${msg.role === "user" ? styles.userMessage : styles.assistantMessage}`}
        >
          <span className={styles.previewRole}>{msg.role === "user" ? "👤" : "🤖"}</span>
          <span className={styles.previewContent}>
            {stripAnsi(msg.content).slice(0, 300)}
            {msg.content.length > 300 ? "…" : ""}
          </span>
        </div>
      ))}
    </div>
  );
}
