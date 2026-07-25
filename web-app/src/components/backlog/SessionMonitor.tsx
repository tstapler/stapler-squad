"use client";
// +feature: backlog:session-monitor

import { useState, useEffect, useRef, useCallback } from "react";
import { useSessionService } from "@/lib/hooks/useSessionService";
import * as styles from "./SessionMonitor.css";

interface SessionMonitorProps {
  sessionId: string;
  sessionRole?: string;
  isRunning: boolean;
}

interface ConversationMessage {
  role: string;
  content: string;
  timestamp?: string;
}

const QUICK_ACTIONS = ["1", "2", "3", "y", "n"];
const POLL_INTERVAL_MS = 5000;
const CONVERSATION_LIMIT = 30;
const TERMINAL_LINES = 60;

export function SessionMonitor({ sessionId, sessionRole, isRunning }: SessionMonitorProps) {
  const { getTerminalSnapshot, writeToSession, getConversationMessages } = useSessionService();

  const [view, setView] = useState<"terminal" | "history">("terminal");
  const [terminalOutput, setTerminalOutput] = useState("");
  const [messages, setMessages] = useState<ConversationMessage[]>([]);
  const [inputValue, setInputValue] = useState("");
  const [sending, setSending] = useState(false);

  const outputRef = useRef<HTMLDivElement>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchTerminal = useCallback(async () => {
    const output = await getTerminalSnapshot(sessionId, TERMINAL_LINES);
    if (output) setTerminalOutput(output);
  }, [sessionId, getTerminalSnapshot]);

  const fetchConversation = useCallback(async () => {
    const msgs = await getConversationMessages(sessionId, CONVERSATION_LIMIT);
    setMessages(msgs);
  }, [sessionId, getConversationMessages]);

  const refresh = useCallback(() => {
    void fetchTerminal();
    void fetchConversation();
  }, [fetchTerminal, fetchConversation]);

  // Reset stale state when sessionId changes
  useEffect(() => {
    setTerminalOutput("");
    setMessages([]);
  }, [sessionId]);

  // Initial load + polling while running
  useEffect(() => {
    void refresh();
    if (isRunning) {
      pollRef.current = setInterval(refresh, POLL_INTERVAL_MS);
    }
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, [isRunning, refresh]);

  // Auto-scroll output to bottom
  useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
  }, [terminalOutput, messages]);

  const handleSend = useCallback(
    async (text: string) => {
      if (!text.trim() || sending) return;
      setSending(true);
      try {
        await writeToSession(sessionId, text, true);
        setInputValue("");
        // Refresh output after a short delay to capture response
        setTimeout(() => void refresh(), 800);
      } finally {
        setSending(false);
      }
    },
    [sessionId, writeToSession, sending, refresh]
  );

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      void handleSend(inputValue);
    }
  };

  return (
    <div className={styles.container}>
      <div className={styles.toolbar}>
        {isRunning && <span className={styles.liveIndicator} aria-hidden="true" />}
        <span className={styles.toolbarTitle}>
          {sessionRole ? `${sessionRole} session` : "session"} · {isRunning ? "running" : "ended"}
        </span>
        <button
          className={`${styles.viewToggle} ${view === "terminal" ? styles.viewToggleActive : ""}`}
          onClick={() => setView("terminal")}
          title="Show terminal output"
        >
          Terminal
        </button>
        <button
          className={`${styles.viewToggle} ${view === "history" ? styles.viewToggleActive : ""}`}
          onClick={() => setView("history")}
          title="Show conversation history"
        >
          History
        </button>
        <a
          className={styles.openLink}
          href={`/?session=${sessionId}`}
          title="Open full terminal"
          aria-label="Open session in full terminal view"
        >
          ↗ Open
        </a>
      </div>

      <div className={styles.outputArea} ref={outputRef} aria-label="Session output" aria-live="polite">
        {view === "history" ? (
          messages.length === 0 ? (
            <div className={styles.emptyState}>No conversation history yet…</div>
          ) : (
            <div className={styles.messageList}>
              {messages.map((msg, i) => (
                <div key={i} className={styles.message}>
                  <span
                    className={`${styles.messageRole} ${
                      msg.role === "user" ? styles.messageRoleUser : styles.messageRoleAssistant
                    }`}
                  >
                    {msg.role}
                  </span>
                  <span className={styles.messageContent}>
                    {truncateContent(msg.content)}
                  </span>
                </div>
              ))}
            </div>
          )
        ) : terminalOutput ? (
          <pre className={styles.terminalOutput}>{terminalOutput}</pre>
        ) : (
          <div className={styles.emptyState}>No output yet…</div>
        )}
      </div>

      <div className={styles.inputRow}>
        <div className={styles.quickActions} aria-label="Quick inputs">
          {QUICK_ACTIONS.map((action) => (
            <button
              key={action}
              className={styles.quickActionButton}
              onClick={() => void handleSend(action)}
              disabled={sending || !isRunning}
              aria-label={`Send "${action}"`}
              title={`Send "${action}"`}
            >
              {action}
            </button>
          ))}
        </div>
        <input
          type="text"
          className={styles.textInput}
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Send input to session…"
          disabled={sending || !isRunning}
          aria-label="Session input"
          data-testid="session-monitor-input"
        />
        <button
          className={styles.sendButton}
          onClick={() => void handleSend(inputValue)}
          disabled={sending || !inputValue.trim() || !isRunning}
          data-testid="session-monitor-send"
        >
          Send
        </button>
      </div>
    </div>
  );
}

// Cap message content at 800 chars to avoid massive tool-use blobs in the monitor
function truncateContent(content: string): string {
  if (!content) return "";
  try {
    // If content is JSON (tool use), show a compact summary
    const parsed = JSON.parse(content);
    if (Array.isArray(parsed)) {
      return parsed
        .map((block: { type?: string; text?: string; name?: string }) => {
          if (block.type === "text") return block.text?.slice(0, 400) ?? "";
          if (block.type === "tool_use") return `[tool: ${block.name}]`;
          if (block.type === "tool_result") return `[tool result]`;
          return JSON.stringify(block).slice(0, 100);
        })
        .filter(Boolean)
        .join("\n");
    }
  } catch {
    // not JSON — plain text
  }
  return content.length > 800 ? content.slice(0, 800) + "…" : content;
}
