// Pure helpers for the Reply card (ADR-010, design/ux.md Surface 12b). The server stamps
// question_id, question_shape and question_options on a replyable question's notification;
// everything else is "Answer in the terminal".

import type { NotificationData } from "@/lib/types/notification";

export const MAX_REPLY_OPTIONS = 7;
export const PROMPT_TRUNCATE_CHARS = 280;

export type ReplyUnavailableCause = "no_proof" | "bad_proof" | "shape" | "path_only";

export type ReplyAvailability =
  | { kind: "replyable"; questionId: string; options: string[]; prompt: string }
  | { kind: "unavailable"; cause: ReplyUnavailableCause };

/** The first 280 characters of a question, and whether anything was cut. */
export function truncatePrompt(prompt: string): { shown: string; truncated: boolean } {
  const chars = Array.from(prompt);
  if (chars.length <= PROMPT_TRUNCATE_CHARS) return { shown: prompt, truncated: false };
  return { shown: chars.slice(0, PROMPT_TRUNCATE_CHARS).join(""), truncated: true };
}

function parseOptions(raw: string | undefined): string[] | null {
  if (!raw) return null;
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed) || parsed.length < 1 || parsed.length > MAX_REPLY_OPTIONS) return null;
    if (!parsed.every((o) => typeof o === "string" && o.trim() !== "")) return null;
    return parsed as string[];
  } catch {
    return null;
  }
}

/**
 * null when the notification is not a question at all; otherwise whether Reply can
 * answer it. A question the server did not register (no question_id) is unavailable.
 */
export function replyAvailability(
  n: Pick<NotificationData, "notificationType" | "metadata" | "message">,
): ReplyAvailability | null {
  if (n.notificationType !== "question") return null;
  const md = n.metadata ?? {};
  const options = parseOptions(md["question_options"]);
  if (md["question_id"] && md["question_shape"] === "single" && options) {
    return { kind: "replyable", questionId: md["question_id"], options, prompt: n.message };
  }
  const cause = md["reply_unavailable"];
  if (cause === "no_proof" || cause === "bad_proof" || cause === "path_only") {
    return { kind: "unavailable", cause };
  }
  return { kind: "unavailable", cause: "shape" };
}

/** True for the two causes that say the session's hook has no valid proof (RP-17). */
export function isHookProofCause(cause: ReplyUnavailableCause): boolean {
  return cause === "no_proof" || cause === "bad_proof";
}

/** The copy of an unavailable question's card. */
export function unavailableCopy(cause: ReplyUnavailableCause): string {
  return isHookProofCause(cause)
    ? "Reply unavailable for this session. Answer in the terminal."
    : "Answer in the terminal";
}

/** The newest unread question of sessionId, preferring the one a deep link named. */
export function pendingQuestionFor(
  history: ReadonlyArray<NotificationData & { isRead: boolean }>,
  sessionId: string,
  preferId?: string | null,
): (NotificationData & { isRead: boolean }) | undefined {
  const mine = history.filter((n) => n.sessionId === sessionId && n.notificationType === "question");
  if (preferId) {
    const named = mine.find((n) => n.id === preferId && !n.isRead);
    if (named) return named;
  }
  return mine.filter((n) => !n.isRead).sort((a, b) => b.timestamp - a.timestamp)[0];
}
