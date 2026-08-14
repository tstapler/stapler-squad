// +feature: chat-refinement-panel

import { useState, useCallback } from "react";
import * as styles from "./ChatRefinementPanel.css";

export interface ChatTurn {
  role: "user" | "assistant";
  text: string;
}

export interface ChatRefinementPanelProps {
  /** Pending clarifying questions from the most recent triage result. This
   * array is fully replaced (not appended to) each time a triage run
   * completes, so only the first entry — the current outstanding question —
   * is surfaced, rather than paginating through a fixed local snapshot. */
  clarifyingQuestions: string[];
  /** Sends one chat turn (a free-text message). Resolves once the turn has
   * been accepted and a new triage run queued — TriggerTriage runs
   * asynchronously, so the item's own triage state (and this component's
   * clarifyingQuestions prop) updates live via the page's existing item
   * subscription once that run actually completes, not synchronously here. */
  onSend: (message: string) => Promise<void>;
}

/**
 * Additive chat surface for multi-turn backlog item refinement — sits
 * alongside TriageReviewPanel's structured refine-feedback form on the
 * Backlog page's item detail view, without modifying that panel.
 */
export function ChatRefinementPanel({ clarifyingQuestions, onSend }: ChatRefinementPanelProps) {
  const [transcript, setTranscript] = useState<ChatTurn[]>([]);
  const [input, setInput] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const currentQuestion = clarifyingQuestions.length > 0 ? clarifyingQuestions[0] : null;

  const handleSend = useCallback(async () => {
    const message = input.trim();
    if (!message || sending) return;
    setSending(true);
    setError(null);
    const answeringQuestion = currentQuestion;
    setTranscript((prev) => [...prev, { role: "user", text: message }]);
    setInput("");
    try {
      await onSend(message);
      setTranscript((prev) => [
        ...prev,
        {
          role: "assistant",
          text: answeringQuestion
            ? "Thanks — re-running triage with your answer. This item will update once it's done."
            : "Got it — re-running triage with your message. This item will update once it's done.",
        },
      ]);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to send message");
    } finally {
      setSending(false);
    }
  }, [input, sending, currentQuestion, onSend]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        void handleSend();
      }
    },
    [handleSend]
  );

  return (
    <div className={styles.panel} data-testid="chat-refinement-panel">
      <h4 className={styles.heading}>Refine via chat</h4>

      {currentQuestion && (
        <div className={styles.questionBanner} data-testid="chat-refinement-question">
          <p className={styles.questionMeta}>
            {clarifyingQuestions.length > 1
              ? `Clarifying question (${clarifyingQuestions.length} pending)`
              : "Clarifying question"}
          </p>
          <p className={styles.questionText}>{currentQuestion}</p>
        </div>
      )}

      {transcript.length > 0 && (
        <div className={styles.transcript} data-testid="chat-refinement-transcript">
          {transcript.map((turn, i) => (
            <div
              key={i}
              className={turn.role === "user" ? styles.turnUser : styles.turnAssistant}
              data-testid={`chat-turn-${turn.role}`}
            >
              {turn.text}
            </div>
          ))}
        </div>
      )}

      {error && <p className={styles.errorText}>{error}</p>}

      <div className={styles.inputRow}>
        <input
          className={styles.input}
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={currentQuestion ? "Answer the question above…" : "Ask for a change, or add more detail…"}
          disabled={sending}
          aria-label="Chat refinement message"
          data-testid="chat-refinement-input"
        />
        <button
          type="button"
          className={styles.sendButton}
          onClick={() => void handleSend()}
          disabled={sending || !input.trim()}
          data-testid="chat-refinement-send"
        >
          {sending ? "Sending…" : "Send"}
        </button>
      </div>
    </div>
  );
}
