"use client";
// +feature: guidance:request-panel

import { useState, useCallback } from "react";
import { useListGuidanceRequestsQuery, useAnswerGuidanceRequestMutation } from "@/lib/api/guidanceApi";
import { getErrorMessage } from "@/lib/utils/connectError";
import * as styles from "./GuidanceRequestPanel.css";

export interface GuidanceRequestPanelProps {
  /** Matches session.v1.GuidanceRequest's scope string. */
  scope: "backlog-item" | "session";
  /** item_id or session_uuid, depending on scope. */
  scopeKey: string;
  /** Heading text; defaults to "Guidance requests". */
  title?: string;
}

interface GuidanceRow {
  id: string;
  questionText: string;
  questionType: string;
  options: string[];
  answer: string;
  status: string;
}

/** Encapsulates the answer-submission mutation + in-flight/error state, split out to keep GuidanceRequestPanel short. */
function useGuidanceAnswering() {
  const [answerGuidanceRequest] = useAnswerGuidanceRequestMutation();
  const [answeringId, setAnsweringId] = useState<string | null>(null);
  const [answerError, setAnswerError] = useState<string | null>(null);

  const submitAnswer = useCallback(
    async (id: string, answer: string) => {
      setAnsweringId(id);
      setAnswerError(null);
      try {
        const result = await answerGuidanceRequest({ id, answer }).unwrap();
        if (!result.applied) {
          setAnswerError("This question was already answered by someone else.");
        }
      } catch (err) {
        setAnswerError(
          getErrorMessage(err, "Failed to submit answer. It may no longer be pending — reload and try again.")
        );
      } finally {
        setAnsweringId(null);
      }
    },
    [answerGuidanceRequest]
  );

  return { answeringId, answerError, submitAnswer };
}

/**
 * GuidanceRequestPanel — the one shared component (AC3) rendering durable
 * guidance requests' pending/answered state. Embedded identically in
 * BacklogItemDetail, TriageReviewPanel, and SessionDetailView rather than
 * forked per view, per project_plans/durable-guidance-request's Phase 5 —
 * that duplication would also trip the jscpd/dupl duplication gates
 * (web-app/.jscpd.json, .golangci.yml).
 *
 * Renders nothing (no empty state) when the scope has no open/recent
 * requests, mirroring TriageReviewPanel's "nothing to show" convention.
 */
export function GuidanceRequestPanel({ scope, scopeKey, title = "Guidance requests" }: GuidanceRequestPanelProps) {
  const { data, error, isLoading } = useListGuidanceRequestsQuery(
    { scope, scopeKey },
    { skip: !scopeKey, pollingInterval: 5000 }
  );
  const { answeringId, answerError, submitAnswer } = useGuidanceAnswering();

  if (!scopeKey || isLoading || error) return null;
  const requests = data?.requests ?? [];
  if (requests.length === 0) return null;

  return (
    <section className={styles.panel} aria-live="polite" data-testid="guidance-request-panel">
      <h3 className={styles.heading}>{title}</h3>
      {answerError && (
        <p className={styles.errorText} role="alert">
          {answerError}
        </p>
      )}
      <ul className={styles.list}>
        {requests.map((r) => (
          <GuidanceRequestItem
            key={r.id}
            request={r}
            busy={answeringId === r.id}
            onSubmit={(answer) => void submitAnswer(r.id, answer)}
          />
        ))}
      </ul>
    </section>
  );
}

function GuidanceRequestItem({
  request,
  busy,
  onSubmit,
}: {
  request: GuidanceRow;
  busy: boolean;
  onSubmit: (answer: string) => void;
}) {
  const isPending = request.status === "pending";
  return (
    <li className={styles.item} data-testid={`guidance-request-${request.id}`}>
      <div className={styles.itemHeader}>
        <span
          className={isPending ? styles.badgePending : styles.badgeAnswered}
          data-testid="guidance-request-status"
        >
          {isPending ? "Pending" : "Answered"}
        </span>
        <p className={styles.questionText}>{request.questionText}</p>
      </div>
      {isPending ? (
        <GuidanceAnswerForm request={request} busy={busy} onSubmit={onSubmit} />
      ) : (
        <p className={styles.answerText}>Answer: {request.answer}</p>
      )}
    </li>
  );
}

interface AnswerFormProps {
  request: GuidanceRow;
  busy: boolean;
  onSubmit: (answer: string) => void;
}

function GuidanceAnswerForm(props: AnswerFormProps) {
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
