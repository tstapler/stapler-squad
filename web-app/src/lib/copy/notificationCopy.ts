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

/** Inline "Approving..." text while a request is in flight. */
export const PENDING_VERB = { approve: "Approving", deny: "Denying", other: "Sending" } as const;

export const decided = (choice: string) => `Decided: ${choice}`;
export const answered = (value: string) => `Answered: ${value}`;
export const alreadyHandled = (value?: string) => (value ? `Already handled: ${value}` : "Already handled");
export const handledByRule = (name: string) => `Handled automatically by rule ${name}`;
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
} as const;
