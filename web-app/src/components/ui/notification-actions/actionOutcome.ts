import { ActionKind, ActionOutcome, ActionTier } from "@/gen/session/v1/notification_action_pb";
import type { InvokeNotificationActionResponse } from "@/gen/session/v1/notification_action_pb";
import {
  ANNOUNCE,
  COPY,
  alreadyHandled,
  answered,
  decided,
  handledByRule,
} from "@/lib/copy/notificationCopy";

export interface OutcomeContext {
  kind: ActionKind;
  tier: ActionTier;
  /** Label of the option the operator chose (answer, decision). */
  chosenLabel?: string;
  /** Set when the approval was auto-handled by a classifier rule. */
  ruleName?: string;
}

export type RetryMode = "none" | "direct" | "confirm" | "countdown";

export interface OutcomeView {
  text: string;
  /** Terminal for this row: the action buttons are gone. */
  settled: boolean;
  /** Short polite-region result, when the outcome has one. */
  announce?: string;
  retry: RetryMode;
  /** Open session is the primary exit (SEND_INDETERMINATE). */
  openPrimary?: boolean;
  refresh?: boolean;
  countdownSeconds?: number;
}

const GONE = { settled: true, retry: "none" } as const;

function applied(ctx: OutcomeContext): OutcomeView {
  switch (ctx.kind) {
    case ActionKind.APPROVE:
      return { ...GONE, text: COPY.approved, announce: COPY.approved };
    case ActionKind.DENY:
      return { ...GONE, text: COPY.denied, announce: COPY.denied };
    case ActionKind.BACKLOG_DECISION: {
      const text = decided(ctx.chosenLabel ?? "");
      return { ...GONE, text, announce: text };
    }
    case ActionKind.ANSWER_QUESTION:
      // A live-pane answer is only "Sent": "Answered" waits for the agent's acknowledgement.
      return ctx.tier === ActionTier.B
        ? { ...GONE, text: COPY.sentWaiting, announce: ANNOUNCE.answerSent }
        : { ...GONE, text: answered(ctx.chosenLabel ?? ""), announce: ANNOUNCE.answerSent };
    case ActionKind.ANSWER_GUIDANCE:
      return { ...GONE, text: answered(ctx.chosenLabel ?? ""), announce: ANNOUNCE.answerSent };
    case ActionKind.CANCEL_WAKE:
      return { ...GONE, text: COPY.promptCancelled, announce: COPY.promptCancelled };
    case ActionKind.CANCEL_PEER_HELP:
      return { ...GONE, text: COPY.requestCancelled, announce: COPY.requestCancelled };
    default:
      return { ...GONE, text: COPY.approved, announce: COPY.approved };
  }
}

function stale(res: InvokeNotificationActionResponse, ctx: OutcomeContext): OutcomeView {
  const approvalKind = ctx.kind === ActionKind.APPROVE || ctx.kind === ActionKind.DENY;
  if (approvalKind && ctx.ruleName) return { ...GONE, text: handledByRule(ctx.ruleName) };
  if (res.staleReason === "dialog_replaced") return { ...GONE, text: COPY.staleReplaced };
  if (res.staleReason === "session_ended") return { ...GONE, text: COPY.sessionEnded };
  return { ...GONE, text: COPY.staleGone };
}

/**
 * Maps a typed outcome to the row's copy and exits (design/ux.md 2.3 and 3.1). The
 * server never reports an expected condition as a Connect error, so this is total
 * over ActionOutcome; anything unknown reads as INVALID.
 */
export function outcomeView(res: InvokeNotificationActionResponse, ctx: OutcomeContext): OutcomeView {
  switch (res.outcome) {
    case ActionOutcome.APPLIED:
      return applied(ctx);
    case ActionOutcome.ALREADY_RESOLVED:
      return { ...GONE, text: alreadyHandled(res.detail || undefined) };
    case ActionOutcome.STALE:
      return stale(res, ctx);
    case ActionOutcome.NOT_SENT:
      return {
        settled: false,
        retry: "direct",
        text: res.detail === "human_active" ? COPY.notSentHumanActive : COPY.notSent,
        announce: ANNOUNCE.notDelivered,
      };
    case ActionOutcome.SEND_INDETERMINATE:
      return { settled: false, retry: "confirm", openPrimary: true, text: COPY.indeterminate };
    case ActionOutcome.RATE_LIMITED:
      return { settled: false, retry: "countdown", text: "", countdownSeconds: Math.max(0, res.retryAfterSeconds) };
    case ActionOutcome.FORBIDDEN:
      return { ...GONE, text: res.detail === "desktop_only" ? COPY.desktopOnly : COPY.forbidden };
    case ActionOutcome.FLAG_OFF:
      return { ...GONE, text: COPY.flagOff };
    default:
      return { settled: false, retry: "none", refresh: true, text: COPY.invalid };
  }
}
