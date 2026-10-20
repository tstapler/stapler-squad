"use client";
// +feature: session:guidance-structured

import { useEffect, useMemo, useRef, type ReactNode } from "react";
import { create } from "@bufbuild/protobuf";
import { ActionKind, ActionOptionSchema, ActionOutcome, type NotificationActionDescriptor } from "@/gen/session/v1/notification_action_pb";
import type { PlainGuidanceRequest } from "@/lib/api/guidanceApi";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import type { ActionSpec, InvokeState } from "@/lib/hooks/useInvokeNotificationAction";
import { announceTransition, deliveryView, snapshotOf, type ClientDelivery, type DeliveryAction, type DeliveryView } from "@/lib/guidance/deliveryView";
import { GuidanceDeliveryLine } from "@/components/guidance/GuidanceDeliveryLine";
import { GuidanceQuestionForm, formatAnswers, type FormQuestion, type QuestionAnswer } from "@/components/guidance/GuidanceQuestionForm";
import { buildSpec } from "./actionSpec";

const isLive = (d?: NotificationActionDescriptor) => d?.kind === ActionKind.ANSWER_QUESTION;
const REASON_READ_ONLY = "read_only";
/** ActionResult.StaleReason (server/services/question_answer_sequence.go) when part of a multi-question answer was typed. */
const ANSWER_ABORTED_PARTIAL = "answer_aborted_partial";

/** What this browser knows about the last attempt, in the vocabulary of the delivery line. */
export function clientDeliveryOf(state: InvokeState): ClientDelivery {
  if (state.status === "pending") return { sending: true };
  if (state.status === "failed") return { clientFailure: true };
  if (state.status !== "settled") return {};
  const { outcome, detail, staleReason } = state.response;
  switch (outcome) {
    case ActionOutcome.APPLIED:
      return { applied: true };
    case ActionOutcome.NOT_SENT:
      return { notSent: detail === "human_active" ? "human_active" : "plain" };
    case ActionOutcome.SEND_INDETERMINATE:
      return { indeterminate: true };
    case ActionOutcome.STALE:
      if (staleReason === ANSWER_ABORTED_PARTIAL) return { abortedPartial: true };
      return { staleReason: staleReason === "dialog_replaced" || staleReason === "session_ended" ? staleReason : "dialog_gone" };
    case ActionOutcome.FORBIDDEN:
      return detail === "desktop_only" ? { desktopOnly: true } : {};
    case ActionOutcome.ALREADY_RESOLVED:
      return detail ? { elsewhere: detail } : {};
    default:
      return {};
  }
}

export const toFormQuestions = (g: PlainGuidanceRequest): FormQuestion[] =>
  (g.questions ?? []).map((q) => ({
    text: q.question,
    header: q.header || undefined,
    multiSelect: q.multiSelect,
    options: q.options.map((o) => ({ label: o.label, description: o.description || undefined })),
  }));

const hasOptions = (q: FormQuestion) => q.options.length > 0;

/** A single-select question: one digit answers it. */
const singleDigitShape = (qs: FormQuestion[]) => qs.length === 1 && !qs[0].multiSelect;

/**
 * The one two-question shape with a recorded key sequence (Spike S1c, mirrors session.MeasuredTabbedShape):
 * a single-select, then a multi-select. It is answered by one Submit carrying every pick.
 */
export const tabbedShape = (qs: FormQuestion[]) => qs.length === 2 && !qs[0].multiSelect && qs[1].multiSelect && qs.every(hasOptions);

/** A keystroke route writes keys: only a single digit or the measured tabbed sequence is safe. */
const keystrokeShape = (qs: FormQuestion[]) => singleDigitShape(qs) || tabbedShape(qs);

/** The answer of a live route: the label for one digit, else one list of labels per question as JSON (EncodeSequencePicks). */
export const liveAnswer = (answers: QuestionAnswer[]): string => (answers.length === 1 ? answers[0].selected[0] : JSON.stringify(answers.map((a) => a.selected)));

export interface GuidanceBodyProps {
  rowId: string;
  guidance: PlainGuidanceRequest;
  descriptor?: NotificationActionDescriptor;
  state: InvokeState;
  /** Offline, Reconnecting or Updating...: controls stay visible but aria-disabled. */
  disabledReason?: string;
  minHeight?: number;
  openControl: ReactNode;
  invoke: (spec: ActionSpec) => void;
  retry: () => void;
  reset: () => void;
  /** Browser-local "Mark as handled" (Unconfirmed rows). */
  onMarkHandled?: () => void;
  /** Reports the row the card shows, so the tray can pull a reopened card back into view. */
  onDeliveryRow?: (view: DeliveryView | null) => void;
}

/**
 * The body of a question card (design/ux.md 3): the shared form, then the one delivery-state line
 * derived from the durable row and this browser's last attempt. Controls stay mounted while the
 * card flips to a final state, so focus is not moved when a terminal answer lands (UX-AC26).
 */
export function GuidanceBody(props: GuidanceBodyProps) {
  const { rowId, guidance, descriptor, state, disabledReason, minHeight, openControl, invoke, retry, reset, onMarkHandled, onDeliveryRow } = props;
  const { announce } = useAnnounce();
  const questions = useMemo(() => toFormQuestions(guidance), [guidance]);
  const client = useMemo(() => clientDeliveryOf(state), [state]);
  const view = deliveryView(snapshotOf(guidance), client);

  const readOnlyReason = descriptor?.unavailableReason === REASON_READ_ONLY;
  const canAnswer = !!descriptor && (descriptor.available || descriptor.unavailableReason === "desktop_only") && !readOnlyReason && (!isLive(descriptor) || keystrokeShape(questions));
  // Once answerable, a card stays a form (locked) when it flips, so the focused control is not removed.
  const wasAnswerable = useRef(false);
  if (canAnswer) wasAnswerable.current = true;
  const answerable = canAnswer || wasAnswerable.current;
  const desktopOnly = descriptor?.unavailableReason === "desktop_only";

  const prevRow = useRef<number | null | undefined>(undefined);
  useEffect(() => {
    const text = announceTransition(prevRow.current, view);
    if (text) announce(text, "polite", `delivery-${rowId}`);
    prevRow.current = view?.row ?? null;
    onDeliveryRow?.(view);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view?.row]);

  const send = (answers: QuestionAnswer[]) => {
    if (!descriptor) return;
    const text = isLive(descriptor) ? liveAnswer(answers) : formatAnswers(answers);
    invoke(buildSpec({ descriptor, option: create(ActionOptionSchema, { label: text }) }));
  };

  const onAction = (action: Exclude<DeliveryAction, "open">) => {
    if (action === "retry") retry();
    else if (action === "retryAnyway") {
      if (state.status === "settled") invoke({ ...state.spec, params: { ...state.spec.params, confirm: true } });
    } else if (action === "markHandled") onMarkHandled?.();
    else {
      reset();
      onMarkHandled?.();
    }
  };

  const line = view && <GuidanceDeliveryLine view={view} onAction={onAction} openControl={openControl} minHeight={minHeight} busy={state.status === "pending"} />;
  const locked = !answerable || desktopOnly || !!view?.locked || state.status === "pending";
  return (
    <GuidanceQuestionForm
      id={guidance.id || rowId}
      questions={questions}
      mode={answerable ? "answer" : "readonly"}
      canTypeText={!!descriptor && !isLive(descriptor)}
      unrenderable={questions.length === 0}
      locked={locked}
      disabledReason={disabledReason}
      onSubmit={send}
      openControl={openControl}
      delivery={line ?? undefined}
      minHeight={minHeight}
    />
  );
}
