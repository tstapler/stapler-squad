"use client";
// +feature: backlog:compose-widget

import { useRef, useState } from "react";
import { useBacklogService, type ParsedBacklogItemDraft } from "@/lib/hooks/useBacklogService";
import * as formStyles from "./BacklogItemForm.css";
import * as styles from "./BacklogComposeWidget.css";

interface BrainstormTurn {
  id: number;
  question: string;
  /** Null once the call finished without a usable draft. */
  draft: ParsedBacklogItemDraft | null;
  pending: boolean;
}

interface BrainstormPanelProps {
  /** Current compose textarea text, sent with each question as context. */
  draftText: string;
  knownRepoPaths: string[];
}

/** Append-only Q&A over parseBacklogItemIntent. Never blocks the compose submit button. */
export function BrainstormPanel({ draftText, knownRepoPaths }: BrainstormPanelProps) {
  const { parseBacklogItemIntent } = useBacklogService();
  const [question, setQuestion] = useState("");
  const [turns, setTurns] = useState<BrainstormTurn[]>([]);
  const nextId = useRef(0);

  const ask = async () => {
    const q = question.trim();
    if (!q) return;
    setQuestion("");
    const id = nextId.current++;
    setTurns((prev) => [...prev, { id, question: q, draft: null, pending: true }]);
    const message = draftText.trim() ? `${q}\n\n${draftText.trim()}` : q;
    const draft = await parseBacklogItemIntent(message, knownRepoPaths);
    setTurns((prev) =>
      prev.map((t) => (t.id === id ? { ...t, draft, pending: false } : t))
    );
  };

  return (
    <div
      className={styles.brainstormPanel}
      data-testid="brainstorm-panel"
      aria-live="polite"
      aria-busy={turns.some((t) => t.pending)}
    >
      {turns.map((t) => (
        <div key={t.id} className={styles.brainstormTurn} data-testid="brainstorm-turn">
          <span className={styles.brainstormQuestion}>{t.question}</span>
          {t.pending && <span>Thinking…</span>}
          {!t.pending && !t.draft && <span>No suggestions — try rephrasing.</span>}
          {t.draft && (
            <>
              {t.draft.description && <span>{t.draft.description}</span>}
              {t.draft.acceptanceCriteria.length > 0 && (
                <>
                  <span>Suggested ACs:</span>
                  <ul>
                    {t.draft.acceptanceCriteria.map((ac, j) => (
                      <li key={j}>{ac}</li>
                    ))}
                  </ul>
                </>
              )}
            </>
          )}
        </div>
      ))}
      <div className={styles.brainstormRow}>
        <input
          type="text"
          className={formStyles.input}
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.metaKey && !e.ctrlKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              e.stopPropagation();
              void ask();
            }
          }}
          placeholder="Ask: what are the ACs? what are the risks?"
          aria-label="Ask the AI about this idea"
          data-testid="brainstorm-input"
        />
        <button
          type="button"
          className={`${formStyles.cancelButton} ${styles.touchTarget}`}
          onClick={() => void ask()}
          disabled={!question.trim()}
          data-testid="brainstorm-ask"
        >
          {/* analytics-exempt */}
          Ask
        </button>
      </div>
    </div>
  );
}
