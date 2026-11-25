"use client";

import { useState } from "react";
import * as styles from "./GuidanceRequestPanel.css";

/** The yes/no, multiple-choice and short-answer forms of a request that carries no structured questions. */
export interface LegacyRequest {
  id: string;
  questionText: string;
  questionType: string;
  options: string[];
}

interface AnswerFormProps {
  request: LegacyRequest;
  busy: boolean;
  onSubmit: (answer: string) => void;
}

export function LegacyAnswerForm(props: AnswerFormProps) {
  switch (props.request.questionType) {
    case "yes-no":
      return <WithFreeForm {...props}><YesNoAnswer {...props} /></WithFreeForm>;
    case "multiple-choice":
      return <WithFreeForm {...props}><MultipleChoiceAnswer {...props} /></WithFreeForm>;
    default:
      return <ShortAnswerForm {...props} />;
  }
}

/** Choice questions still accept a typed answer: the agent's options are suggestions, not the only valid replies. */
function WithFreeForm({ children, ...props }: AnswerFormProps & { children: React.ReactNode }) {
  return (
    <>
      {children}
      <ShortAnswerForm {...props} />
    </>
  );
}

function YesNoAnswer({ request, busy, onSubmit }: AnswerFormProps) {
  return (
    <div className={styles.actions}>
      <button
        type="button"
        className={styles.answerButton}
        disabled={busy}
        onClick={() => onSubmit("yes")}
        data-testid={`guidance-answer-yes-${request.id}`}
      >
        Yes
      </button>
      <button
        type="button"
        className={styles.answerButton}
        disabled={busy}
        onClick={() => onSubmit("no")}
        data-testid={`guidance-answer-no-${request.id}`}
      >
        No
      </button>
    </div>
  );
}

function MultipleChoiceAnswer({ request, busy, onSubmit }: AnswerFormProps) {
  return (
    <div className={styles.actions}>
      {request.options.map((opt) => (
        <button
          key={opt}
          type="button"
          className={styles.answerButton}
          disabled={busy}
          onClick={() => onSubmit(opt)}
          data-testid={`guidance-answer-option-${request.id}-${opt}`}
        >
          {opt}
        </button>
      ))}
    </div>
  );
}

function ShortAnswerForm({ request, busy, onSubmit }: AnswerFormProps) {
  const [value, setValue] = useState("");
  return (
    <form
      className={styles.shortAnswerForm}
      onSubmit={(e) => {
        e.preventDefault();
        const trimmed = value.trim();
        if (trimmed) onSubmit(trimmed);
      }}
    >
      <textarea
        className={styles.shortAnswerInput}
        value={value}
        rows={3}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          // Enter inserts a newline; Ctrl/Cmd+Enter submits.
          if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && !e.nativeEvent.isComposing && value.trim()) {
            e.preventDefault();
            onSubmit(value.trim());
          }
        }}
        placeholder="Or type a free-form answer… (Ctrl+Enter to send)"
        disabled={busy}
        aria-label={`Answer for: ${request.questionText}`}
        data-testid={`guidance-answer-input-${request.id}`}
      />
      <button
        type="submit"
        className={styles.answerButton}
        disabled={busy || !value.trim()}
        data-testid={`guidance-answer-submit-${request.id}`}
      >
        Submit
      </button>
    </form>
  );
}
