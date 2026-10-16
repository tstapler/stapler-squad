"use client";

import { useCallback, useEffect, useReducer, useRef } from "react";
import { createClient, ConnectError, Code } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import {
  ReplyOutcome,
  ReplyToPendingQuestionRequestSchema,
  SessionService,
} from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { useNotificationConnectivity } from "@/lib/hooks/useNotificationConnectivity";

// The Reply state machine (ux.md Surface 12b). The response enum is the only signaling
// channel for expected outcomes; Connect errors are InvalidArgument, Internal and
// PermissionDenied only.

export type ReplyPhase =
  | { phase: "choosing" }
  | { phase: "sending"; option: number }
  | { phase: "sent"; option: number; at: number }
  | { phase: "indeterminate"; option: number }
  | { phase: "noPending" }
  | { phase: "stalePrompt" }
  | { phase: "notWaiting"; option: number }
  | { phase: "rateLimited"; option: number; seconds: number }
  | { phase: "invalid" }
  | { phase: "retryable"; option: number }
  | { phase: "refused" }
  | { phase: "hidden" };

export const CHOOSING: ReplyPhase = { phase: "choosing" };

export const REPLY_COPY = {
  noPending: "This question is no longer waiting. You may have answered it elsewhere.",
  stalePrompt: "This question is no longer on screen. You may have answered it already. Check the terminal.",
  notWaiting: "The question isn't showing yet - try again in a moment",
  invalid: "That option is not available. Check the terminal.",
  retry: "Could not send - Retry",
  indeterminate: "Sent? Check the terminal output to confirm",
  refused: "This device is not allowed to reply",
  offline: "Offline - reconnect to reply",
  footer: "Sent to the background session's terminal once. This reply is logged.",
} as const;

/** Maps a server outcome to the next phase. `now` is injected so tests do not read a clock. */
export function phaseForOutcome(outcome: ReplyOutcome, option: number, retryAfterSeconds: number, now: number): ReplyPhase {
  switch (outcome) {
    case ReplyOutcome.SENT:
      return { phase: "sent", option, at: now };
    case ReplyOutcome.SEND_INDETERMINATE:
      return { phase: "indeterminate", option };
    case ReplyOutcome.NO_PENDING:
      return { phase: "noPending" };
    case ReplyOutcome.STALE_PROMPT:
      return { phase: "stalePrompt" };
    case ReplyOutcome.NOT_WAITING:
      return { phase: "notWaiting", option };
    case ReplyOutcome.RATE_LIMITED:
      return { phase: "rateLimited", option, seconds: Math.max(1, retryAfterSeconds) };
    case ReplyOutcome.NOT_HIDDEN:
    case ReplyOutcome.DISABLED:
      return { phase: "hidden" };
    case ReplyOutcome.NOT_SENT:
    default:
      return { phase: "retryable", option };
  }
}

/** Maps a thrown RPC error to a phase: only InvalidArgument and PermissionDenied are special. */
export function phaseForError(err: unknown, option: number): ReplyPhase {
  const code = ConnectError.from(err).code;
  if (code === Code.InvalidArgument) return { phase: "invalid" };
  if (code === Code.PermissionDenied) return { phase: "refused" };
  return { phase: "retryable", option };
}

/** The assertive-or-polite announcement of a phase (the card has no live role). */
export function announcementFor(p: ReplyPhase, optionLabel: (n: number) => string, time: (ms: number) => string):
  | { text: string; politeness: "polite" | "assertive" }
  | null {
  switch (p.phase) {
    case "sent":
      return { text: `Sent: ${p.option}. ${optionLabel(p.option)} at ${time(p.at)} - the question closed`, politeness: "polite" };
    case "indeterminate":
      return { text: REPLY_COPY.indeterminate, politeness: "assertive" };
    case "retryable":
      return { text: REPLY_COPY.retry, politeness: "assertive" };
    case "noPending":
      return { text: REPLY_COPY.noPending, politeness: "polite" };
    case "stalePrompt":
      return { text: REPLY_COPY.stalePrompt, politeness: "assertive" };
    case "notWaiting":
      return { text: REPLY_COPY.notWaiting, politeness: "assertive" };
    case "invalid":
      return { text: REPLY_COPY.invalid, politeness: "assertive" };
    case "refused":
      return { text: REPLY_COPY.refused, politeness: "assertive" };
    case "rateLimited":
      return { text: `Please wait ${p.seconds} seconds`, politeness: "polite" };
    default:
      return null;
  }
}

type Action = { type: "set"; phase: ReplyPhase };
const reducer = (_: ReplyPhase, a: Action): ReplyPhase => a.phase;

export interface UseReplyToQuestionArgs {
  sessionId: string;
  questionId: string;
  /** Option labels, index 0 is option 1. */
  options: readonly string[];
  /** Called once when a reply is SENT, to mark the notification read. */
  onSent?: () => void;
  /** Clock, replaced in tests. */
  now?: () => number;
  /** Replaced in tests; defaults to crypto.randomUUID. */
  newReplyId?: () => string;
  /** Replaced in tests; defaults to the ConnectRPC client. */
  sendReply?: (req: { sessionId: string; questionId: string; replyText: string; replyId: string }) => Promise<{
    outcome: ReplyOutcome;
    retryAfterSeconds: number;
  }>;
}

export interface UseReplyToQuestion {
  state: ReplyPhase;
  offline: boolean;
  /** Tap an option (1-based). Ignored unless the card is accepting taps. */
  choose: (option: number) => void;
  /** Re-send the last option with the same reply_id (only offered where that is safe). */
  retry: () => void;
}

export function useReplyToQuestion(args: UseReplyToQuestionArgs): UseReplyToQuestion {
  const { sessionId, questionId, options, onSent } = args;
  const now = args.now ?? Date.now;
  const [state, dispatch] = useReducer(reducer, CHOOSING);
  const { isOffline } = useNotificationConnectivity();
  const { announce } = useAnnounce();
  const inFlight = useRef(false);
  const attempt = useRef<{ replyId: string; option: number } | null>(null);
  const announced = useRef<ReplyPhase | null>(null);

  const send =
    args.sendReply ??
    (async (req: { sessionId: string; questionId: string; replyText: string; replyId: string }) => {
      // Built on the first tap, so mounting the card creates no transport.
      const client = createClient(SessionService, getConnectTransport());
      // One-shot write from an explicit tap. It is deliberately not tied to the card's
      // lifetime: dropping the request on unmount could not take the digit back, and the
      // server answers a retried reply_id from its cached result.
      // abort-signal-exempt
      const res = await client.replyToPendingQuestion(create(ReplyToPendingQuestionRequestSchema, req));
      return { outcome: res.outcome, retryAfterSeconds: res.retryAfterSeconds };
    });
  const newId = args.newReplyId ?? (() => crypto.randomUUID());

  const submit = useCallback(
    async (option: number, replyId: string) => {
      inFlight.current = true;
      dispatch({ type: "set", phase: { phase: "sending", option } });
      let next: ReplyPhase;
      try {
        const res = await send({ sessionId, questionId, replyText: String(option), replyId });
        next = phaseForOutcome(res.outcome, option, res.retryAfterSeconds, now());
      } catch (err) {
        next = phaseForError(err, option);
      } finally {
        inFlight.current = false;
      }
      dispatch({ type: "set", phase: next });
      if (next.phase === "sent") onSent?.();
    },
    // send/newId close over stable args; the deps below are the ones that identify the question
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sessionId, questionId, onSent],
  );

  const choose = useCallback(
    (option: number) => {
      if (inFlight.current || isOffline) return; // the first tap disables every button (RP-3, RP-18)
      if (option < 1 || option > options.length) return;
      if (!attempt.current || attempt.current.option !== option) attempt.current = { replyId: newId(), option };
      void submit(option, attempt.current.replyId);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [isOffline, options.length, submit],
  );

  const retry = useCallback(() => {
    if (inFlight.current || isOffline || !attempt.current) return;
    void submit(attempt.current.option, attempt.current.replyId);
  }, [isOffline, submit]);

  // Rate limited: keep the options, re-enable them when the wait ends.
  useEffect(() => {
    if (state.phase !== "rateLimited") return;
    const t = setTimeout(() => dispatch({ type: "set", phase: CHOOSING }), state.seconds * 1000);
    return () => clearTimeout(t);
  }, [state]);

  // One announcement per phase change, through the Announcer (the card has no live role).
  useEffect(() => {
    if (announced.current === state) return;
    announced.current = state;
    const a = announcementFor(
      state,
      (n) => options[n - 1] ?? "",
      (ms) => new Date(ms).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    );
    if (a) announce(a.text, a.politeness, `reply:${questionId}`);
  }, [state, options, questionId, announce]);

  return { state, offline: isOffline, choose, retry };
}
