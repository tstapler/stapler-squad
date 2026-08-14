// +feature: chat-refinement-panel

import { useState, useCallback, useMemo } from "react";
import * as styles from "./ChatRefinementPanel.css";

export interface ChatTurn {
  role: "user" | "assistant";
  text: string;
}

export interface ChatRefinementPanelProps {
  /** Pending clarifying questions from the most recent triage result, surfaced
   * one at a time rather than as a batch dump. */
  clarifyingQuestions: string[];
  /** Sends one chat turn (a free-text message) and returns once the
   * resulting triage run has completed and the item has been refreshed. */
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
  const [questionIndex, setQuestionIndex] = useState(0);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const currentQuestion = useMemo(
    () => (questionIndex < clarifyingQuestions.length ? clarifyingQuestions[questionIndex] : null),
    [clarifyingQuestions, questionIndex]
  );

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
      if (answeringQuestion) {
        setQuestionIndex((i) => i + 1);
        setTranscript((prev) => [...prev, { role: "assistant", text: "Got it — thanks for clarifying." }]);
      } else {
        setTranscript((prev) => [...prev, { role: "assistant", text: "Refined the item based on your message." }]);
      }
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
            Clarifying question {questionIndex + 1} of {clarifyingQuestions.length}
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
