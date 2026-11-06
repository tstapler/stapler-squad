import { flagOffMessage } from "@/lib/hooks/useFeatureFlagValue";

/**
 * The one vocabulary module for inline notification actions (design/ux.md section 0,
 * 2.3 and 3.1). UI strings only; internal identifiers keep their names. A test
 * (copyGuard.test.ts) scans every export for the banned terms.
 */

export const COPY = {
  approve: "Approve",
  deny: "Deny",
  open: "Open",
  openSession: "Open session",
  back: "Back",
  retry: "Retry",
  refresh: "Refresh",
  copy: "Copy",
  more: "More",
  sendDenial: "Send denial",
  denyReasonLabel: "Reason (optional)",
  confirmMerge: "Confirm merge",
  createRule: "Create rule",
  createRuleMenu: "Create rule...",
  keep: "Keep",
  cancelPrompt: "Cancel prompt",
  cancelRequest: "Cancel request",
  retryAnyway: "Retry anyway",
  retryAnywayPrompt: "Retry anyway?",
  approved: "Approved",
  denied: "Denied",
  sentWaiting: "Sent - waiting for the agent",
  flagOff: flagOffMessage,
  invalid: "That action isn't valid anymore. Refresh.",
  desktopOnly: "Answer from desktop",
  resumeFromDesktop: "Resume from desktop",
  staleGone: "Already handled in the session. No action needed.",
  staleReplaced: "The session is now asking something else. Open session to answer it.",
  sessionEnded: "Session ended before this was answered.",
  notSent: "Not sent. Nothing was typed into the session. Retry",
  notSentHumanActive: "Not sent: someone is typing in that session. Retry when they are done.",
  indeterminate: "Sent, but we couldn't confirm it arrived. Check the session before retrying.",
  transient: "Couldn't send your action. Retry",
  cantShowCommand: "Can't show this command. Open session to review it.",
  commandRegionLabel: "Command, scrollable",
  forbidden: "This action isn't available here. Open session.",
  offline: "Offline",
  promptCancelled: "Prompt cancelled",
  requestCancelled: "Request cancelled",
  cancelPromptQuestion: "Cancel this prompt?",
  cancelRequestQuestion: "Cancel this request?",
  dismiss: "Dismiss",
  copied: "Copied",
  retryAnywayConfirm: "Retry",
} as const;

/**
 * The delivery-state line of a question card (design/ux.md 3.1; UX-AC23). Rows 4 and 13
 * ("Answered: {value}", "Answered elsewhere: {value}") are the `answered` and `answeredElsewhere`
 * helpers below; the rest of the 17 settled-state strings, and the human_active variant of row 5
 * (`COPY.notSentHumanActive`), are here and in `COPY`.
 */
export const DELIVERY_COPY = {
  sending: "Sending...",
  unconfirmed: "Sent, but the agent hasn't confirmed yet. Open session.",
  unconfirmedChip: "Unconfirmed",
  clientFailure: "Couldn't send your answer. Retry",
  savedStopped: "Your answer is saved. The agent can read it once you resume the session.",
  savedArchived: "This session is archived. Your answer will be saved but not delivered unless you resume it.",
  aborted: "Part of your answer was sent, then the prompt changed. Open session to finish.",
  stillShowing: "Sent, but the question is still showing. Check the session before retrying.",
  alreadyAnsweredReload: "Already answered - reload to see the answer",
  sentSessionEnded: "Sent, but the session ended before confirming it.",
  markHandled: "Mark as handled",
  markHandledPrompt: "Mark as handled in this browser?",
  markHandledConfirm: "Mark handled",
  dismissUnfinishedPrompt: "Dismiss without finishing?",
  dismissUnfinishedConfirm: "Dismiss",
} as const;

/** Labels and helper text of the question form (design/ux.md 3.1 and 3.3). */
export const QUESTION_FORM_COPY = {
  submitAnswers: "Submit answers",
  clear: "Clear",
  send: "Send",
  otherLabel: "Other",
  chooseAtLeastOne: "Choose at least one option",
  viewFullPrompt: "View full prompt in session",
  answerInSession: "Answer in the session",
  noDeliveryYet: "Delivery: (none until sent)",
  questionOf: (n: number, total: number) => `Question ${n} of ${total}`,
  chooseAny: "(choose any)",
  selectedCount: (n: number) => `${n} selected`,
  answerToContinue: (n: number) => `Answer question ${n} to continue`,
  answerFor: (question: string) => `Answer: ${question}`,
  savedForLater: (n: number) => `Saved for later (${n})`,
  savedStopped: "stopped",
  savedArchived: "archived",
} as const;

/** Inline "Approving..." text while a request is in flight. */
export const PENDING_VERB = { approve: "Approving", deny: "Denying", other: "Sending" } as const;

export const decided = (choice: string) => `Decided: ${choice}`;
export const answered = (value: string) => `Answered: ${value}`;
export const answeredElsewhere = (summary: string) => `Answered elsewhere: ${summary}`;
export const alreadyHandled = (value?: string) => (value ? `Already handled: ${value}` : "Already handled");
export const handledByRule = (name: string) => `Handled automatically by rule ${name}`;
export const moreWaiting = (n: number) => `${n} more waiting.`;
export const rateLimited = (seconds: number) => `Too many actions. Try again in ${seconds}s.`;
export const pendingLabel = (verb: string) => `${verb}...`;
export const mergePrompt = (pr?: string) => (pr ? `Merge PR #${pr}?` : "Merge this PR?");
export const createRulePrompt = (what: string) => `Create a rule that auto-approves ${what}?`;
export const characterCount = (n: number) => `${n.toLocaleString("en-US")} characters`;
export const cancelPrompt = (kind: "prompt" | "request") =>
  kind === "prompt" ? COPY.cancelPrompt : COPY.cancelRequest;

/** Polite-region result for a settled outcome ("Approved", "Answer sent"). */
export const ANNOUNCE = {
  applied: (text: string) => text,
  answerSent: "Answer sent",
  notDelivered: "Answer not delivered",
  stillShowing: "Answer sent, question still showing",
  notConfirmed: "Answer not confirmed",
  partlySent: "Answer partly sent",
  handledInSession: "Already handled in the session. No action needed.",
} as const;

/** The three consequential taps that wait 3 s behind an Undo (design/ux.md 2.4), keyed by action. */
export type HoldKey = "approve" | "rework" | "accept";

export const HOLD_COPY: Record<HoldKey, { button: string; verbing: string; pending: string; undone: string; cancelled: string }> = {
  approve: {
    button: "Approve",
    verbing: "Approving",
    pending: "Approval pending",
    undone: "Approval undone",
    cancelled: "Approval cancelled when you left. Tap Approve to try again.",
  },
  rework: {
    button: "Send back",
    verbing: "Sending back",
    pending: "Send back pending",
    undone: "Send back undone",
    cancelled: "Send back cancelled when you left. Tap Send back to try again.",
  },
  accept: {
    button: "Mark done",
    verbing: "Marking done",
    pending: "Mark done pending",
    undone: "Mark done undone",
    cancelled: "Mark done cancelled when you left. Tap Mark done to try again.",
  },
};

export const holdCountdown = (verbing: string, seconds: number) => `${verbing} in ${seconds}s`;
export const UNDO_LABEL = "Undo";
