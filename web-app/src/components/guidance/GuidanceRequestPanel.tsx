"use client";
// +feature: guidance:request-panel

import { useState, useCallback } from "react";
import { useListGuidanceRequestsQuery, useAnswerGuidanceRequestMutation } from "@/lib/api/guidanceApi";
import { getErrorMessage } from "@/lib/utils/connectError";
import type { PlainGuidanceRequest } from "@/lib/api/guidanceApi";
import { deliveryView, snapshotOf } from "@/lib/guidance/deliveryView";
import { GuidanceDeliveryLine } from "./GuidanceDeliveryLine";
import { GuidanceQuestionForm, formatAnswers } from "./GuidanceQuestionForm";
import { LegacyAnswerForm } from "./LegacyAnswerForm";
import * as styles from "./GuidanceRequestPanel.css";

export interface GuidanceRequestPanelProps {
  /** Matches session.v1.GuidanceRequest's scope string. */
  scope: "backlog-item" | "session";
  /** item_id or session_uuid, depending on scope. */
  scopeKey: string;
  /** Heading text; defaults to "Guidance requests". */
  title?: string;
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
  request: PlainGuidanceRequest;
  busy: boolean;
  onSubmit: (answer: string) => void;
}) {
  const isPending = request.status === "pending";
  if ((request.questions?.length ?? 0) > 0) return <StructuredRequestItem request={request} busy={busy} onSubmit={onSubmit} />;
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
        <LegacyAnswerForm request={request} busy={busy} onSubmit={onSubmit} />
      ) : (
        <p className={styles.answerText}>Answer: {request.answer}</p>
      )}
    </li>
  );
}


/** A request that carries structured questions: the shared form, then its delivery-state line. */
function StructuredRequestItem({ request, busy, onSubmit }: { request: PlainGuidanceRequest; busy: boolean; onSubmit: (answer: string) => void }) {
  const questions = (request.questions ?? []).map((q) => ({
    text: q.question,
    header: q.header || undefined,
    multiSelect: q.multiSelect,
    options: q.options.map((o) => ({ label: o.label, description: o.description || undefined })),
  }));
  const view = deliveryView(snapshotOf(request), { sending: busy });
  const pending = request.status === "pending";
  return (
    <li className={styles.item} data-testid={`guidance-request-${request.id}`}>
      <div className={styles.itemHeader}>
        <span className={pending ? styles.badgePending : styles.badgeAnswered} data-testid="guidance-request-status">
          {pending ? "Pending" : "Answered"}
        </span>
        {!pending && <p className={styles.questionText}>{request.questionText}</p>}
      </div>
      <GuidanceQuestionForm
        id={request.id}
        questions={questions}
        mode={pending ? "answer" : "readonly"}
        canTypeText
        locked={busy || (view?.locked ?? false)}
        openControl={null}
        onSubmit={(answers) => onSubmit(formatAnswers(answers))}
        delivery={view && <GuidanceDeliveryLine view={view} onAction={() => {}} openControl={null} />}
      />
    </li>
  );
}
