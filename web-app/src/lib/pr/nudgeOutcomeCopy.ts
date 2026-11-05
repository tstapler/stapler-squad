import { NudgeOutcome } from "@/gen/session/v1/github_user_pb";

export const PAUSED_TEXT = "Session paused. Open it to resume";
export const NOT_RUNNING_TEXT = "Session is not running or was deleted. Open it to restart it.";
const BUSY_FALLBACK_TEXT = "Session can't take a request right now. Open it to continue.";

export interface NudgeMessage {
  text: string;
  role: "status" | "alert";
  openSessionId?: string;
}

/** Copy for each outcome; the `never` default fails the build when the proto gains an outcome. */
export function outcomeMessage(outcome: NudgeOutcome, detail: string, sessionId: string): NudgeMessage {
  const status = (text: string, withLink = false): NudgeMessage => ({
    text,
    role: "status",
    openSessionId: withLink ? sessionId : undefined,
  });
  switch (outcome) {
    case NudgeOutcome.DELIVERED:
      return status(`Request sent to ${sessionId}`, true);
    case NudgeOutcome.BUSY:
      return status(detail || BUSY_FALLBACK_TEXT, true);
    case NudgeOutcome.PAUSED:
      return status(detail || PAUSED_TEXT, true);
    case NudgeOutcome.NOT_RUNNING:
      return status(detail || NOT_RUNNING_TEXT, true);
    case NudgeOutcome.DUPLICATE:
      return status("Already requested in the last minute", true);
    case NudgeOutcome.NOTHING_TO_FIX:
      return status("Nothing to fix right now");
    case NudgeOutcome.SESSION_NOT_LINKED:
      return status("Session no longer linked");
    case NudgeOutcome.PR_NOT_FOUND:
      return status("PR not found (closed or moved?)");
    case NudgeOutcome.UNSPECIFIED:
      return status("Unexpected response. Try again.");
    default: {
      const unhandled: never = outcome;
      return status(`Unexpected response (${String(unhandled)})`);
    }
  }
}
