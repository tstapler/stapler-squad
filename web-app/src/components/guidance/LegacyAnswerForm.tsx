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
      return <YesNoAnswer {...props} />;
    case "multiple-choice":
      return <MultipleChoiceAnswer {...props} />;
    default:
      return <ShortAnswerForm {...props} />;
  }
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
      <input
        type="text"
        className={styles.shortAnswerInput}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        placeholder="Type your answer…"
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
