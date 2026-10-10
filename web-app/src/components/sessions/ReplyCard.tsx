// +feature: session-reply-card
"use client";

import { useId, useState } from "react";
import type { NotificationData } from "@/lib/types/notification";
import {
  REPLY_COPY,
  useReplyToQuestion,
  type ReplyPhase,
  type UseReplyToQuestionArgs,
} from "@/lib/hooks/useReplyToQuestion";
import { replyAvailability, truncatePrompt, unavailableCopy } from "@/lib/reply/replyQuestion";
import * as styles from "./ReplyCard.css";

export interface ReplyCardProps {
  sessionId: string;
  /** The question notification (metadata carries question_id, question_shape, question_options). */
  notification: Pick<NotificationData, "id" | "notificationType" | "metadata" | "message">;
  /** Marks the notification read once a reply is SENT. */
  onSent?: () => void;
  /** Opens the terminal output; offered where the card says "check the terminal". */
  onViewOutput?: () => void;
  /** Test seams of the hook. */
  hookOverrides?: Pick<UseReplyToQuestionArgs, "sendReply" | "newReplyId" | "now">;
}

const NEEDS_VIEW_OUTPUT: ReadonlySet<ReplyPhase["phase"]> = new Set(["indeterminate", "stalePrompt", "invalid"]);

/**
 * Audited one-tap answer to a hidden session's pending single-select question
 * (ADR-010). One numbered button per option label; no text field, so no soft
 * keyboard, IME or Enter path exists (RP-2, RP-12, RP-18). Anything the server
 * cannot answer renders "Answer in the terminal".
 */
export function ReplyCard({ sessionId, notification, onSent, onViewOutput, hookOverrides }: ReplyCardProps) {
  const availability = replyAvailability(notification);
  if (!availability) return null;
  if (availability.kind === "unavailable") {
    return <UnavailableCard copy={unavailableCopy(availability.cause)} onViewOutput={onViewOutput} />;
  }
  return (
    <ReplyChoices
      sessionId={sessionId}
      questionId={availability.questionId}
      options={availability.options}
      prompt={availability.prompt}
      onSent={onSent}
      onViewOutput={onViewOutput}
      hookOverrides={hookOverrides}
    />
  );
}

function UnavailableCard({ copy, onViewOutput }: { copy: string; onViewOutput?: () => void }) {
  const headingId = useId();
  return (
    <section className={styles.card} role="group" aria-labelledby={headingId} data-testid="reply-card" data-state="unavailable">
      <h2 id={headingId} className={styles.heading}>
        Claude has a question
      </h2>
      <p className={styles.status} data-testid="reply-unavailable">
        {copy}
      </p>
      {onViewOutput && (
        <button type="button" className={styles.actionButton} onClick={onViewOutput} data-testid="reply-view-output">
          View output
        </button>
      )}
    </section>
  );
}

interface ReplyChoicesProps {
  sessionId: string;
  questionId: string;
  options: string[];
  prompt: string;
  onSent?: () => void;
  onViewOutput?: () => void;
  hookOverrides?: ReplyCardProps["hookOverrides"];
}

function ReplyChoices({ sessionId, questionId, options, prompt, onSent, onViewOutput, hookOverrides }: ReplyChoicesProps) {
  const headingId = useId();
  const [expanded, setExpanded] = useState(false);
  const reply = useReplyToQuestion({ sessionId, questionId, options, onSent, ...hookOverrides });
  const { state } = reply;
  const { shown, truncated } = truncatePrompt(prompt);

  if (state.phase === "hidden") return null; // NOT_HIDDEN and DISABLED: the card goes away

  const choosing = state.phase === "choosing" || state.phase === "retryable" || state.phase === "notWaiting" || state.phase === "rateLimited";
  const settled = state.phase === "sent" || state.phase === "indeterminate";
  const buttonsDisabled = state.phase === "sending" || state.phase === "rateLimited" || reply.offline;
  const chosen = "option" in state ? state.option : undefined;

  return (
    <section
      className={styles.card}
      role="group"
      aria-labelledby={headingId}
      aria-busy={state.phase === "sending" ? "true" : undefined}
      data-testid="reply-card"
      data-state={state.phase}
    >
      <h2 id={headingId} className={styles.heading}>
        Claude has a question
      </h2>
      <p className={styles.prompt} data-testid="reply-prompt">
        {expanded ? prompt : shown}
        {truncated && !expanded ? "…" : ""}
      </p>
      {truncated && (
        <button
          type="button"
          className={styles.disclosure}
          aria-expanded={expanded}
          onClick={() => setExpanded((v) => !v)}
          data-testid="reply-show-full"
        >
          {expanded ? "Hide full prompt" : "Show full prompt"}
        </button>
      )}

      {(choosing || state.phase === "sending" || settled) && (
        <div className={styles.options}>
          {options.map((label, i) => {
            const n = i + 1;
            if (state.phase === "sent" && n !== chosen) return null; // buttons are removed on success
            return (
              <button
                key={n}
                type="button"
                className={styles.option}
                data-testid={`reply-option-${n}`}
                disabled={buttonsDisabled || settled}
                aria-pressed={chosen === n ? "true" : undefined}
                onClick={() => reply.choose(n)}
              >
                {n}. {label}
              </button>
            );
          })}
        </div>
      )}

      <Status state={state} options={options} offline={reply.offline} onRetry={reply.retry} />

      {onViewOutput && NEEDS_VIEW_OUTPUT.has(state.phase) && (
        <button type="button" className={styles.actionButton} onClick={onViewOutput} data-testid="reply-view-output">
          View output
        </button>
      )}
      <p className={styles.footer}>{REPLY_COPY.footer}</p>
    </section>
  );
}

function Status({
  state,
  options,
  offline,
  onRetry,
}: {
  state: ReplyPhase;
  options: string[];
  offline: boolean;
  onRetry: () => void;
}) {
  const retryButton = (
    <button type="button" className={styles.actionButton} onClick={onRetry} disabled={offline} data-testid="reply-retry">
      Retry
    </button>
  );
  if (offline && state.phase !== "sent" && state.phase !== "indeterminate") {
    return (
      <p className={styles.status} data-testid="reply-status">
        {REPLY_COPY.offline}
      </p>
    );
  }
  switch (state.phase) {
    case "sent":
      return (
        <p className={styles.status} data-testid="reply-receipt" tabIndex={-1} ref={(el) => el?.focus()}>
          Sent: {state.option}. {options[state.option - 1]} at{" "}
          {new Date(state.at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })} - the question closed
        </p>
      );
    case "indeterminate":
      return (
        <p className={styles.status} data-testid="reply-status">
          {REPLY_COPY.indeterminate}
        </p>
      );
    case "noPending":
      return (
        <p className={styles.status} data-testid="reply-status">
          {REPLY_COPY.noPending}
        </p>
      );
    case "stalePrompt":
      return (
        <p className={styles.status} data-testid="reply-status">
          {REPLY_COPY.stalePrompt}
        </p>
      );
    case "invalid":
      return (
        <p className={styles.status} data-testid="reply-status">
          {REPLY_COPY.invalid}
        </p>
      );
    case "refused":
      return (
        <p className={styles.status} data-testid="reply-status">
          {REPLY_COPY.refused}
        </p>
      );
    case "notWaiting":
      return (
        <>
          <p className={styles.status} data-testid="reply-status">
            {REPLY_COPY.notWaiting}
          </p>
          {retryButton}
        </>
      );
    case "retryable":
      return (
        <>
          <p className={styles.status} data-testid="reply-status">
            {REPLY_COPY.retry.replace(" - Retry", "")}
          </p>
          {retryButton}
        </>
      );
    case "rateLimited":
      return (
        <p className={styles.status} data-testid="reply-status">
          Please wait {state.seconds} seconds
        </p>
      );
    default:
      return null;
  }
}
