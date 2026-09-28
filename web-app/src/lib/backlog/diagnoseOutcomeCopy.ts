import type { DiagnoseDispatchProto } from "@/gen/session/v1/diagnose_pb";

/**
 * Human copy for a SkippedSafetyGate outcome's `safety_gate_reason` -- always
 * names the specific reason, never a generic "skipped" (design/ux.md Surface
 * 5/6). Mirrors `session/diagnose.SafetyGateReason`'s raw string values
 * exactly (session/diagnose/outcome.go is the source of truth); the default
 * branch still names whatever raw reason the backend sent rather than
 * falling back to "skipped" alone, so an as-yet-unmapped reason added on the
 * Go side degrades gracefully instead of becoming generic.
 */
export function safetyGateReasonCopy(reason: string | undefined): string {
  switch (reason) {
    case "not_idle":
      return "nudge skipped (session wasn't idle)";
    case "identity_mismatch_instance":
    case "identity_mismatch_tmux_marker":
      return "nudge skipped (identity check failed)";
    case "nudge_cap_reached":
      return "nudge skipped (nudge cap reached)";
    case "nudge_cooldown_active":
      return "nudge skipped (cooldown active)";
    case "nudge_execution_disabled":
      return "nudge skipped (nudging is disabled)";
    case "duplicate_write_attempt_for_dispatch":
      return "nudge skipped (a nudge write was already attempted for this dispatch)";
    default:
      return reason ? `nudge skipped (${reason.replace(/_/g, " ")})` : "nudge skipped (safety gate)";
  }
}

export interface DiagnoseOutcomeCopy {
  icon: string;
  /** Lowercase clause with no leading "Diagnosed <time> ago --" and no trailing period -- callers compose the full sentence. */
  text: string;
}

type DiagnoseOutcomeCopyInput = Pick<DiagnoseDispatchProto, "outcomeKind" | "safetyGateReason" | "failureReason">;

/**
 * The one-line icon+text summary for a completed dispatch's outcome --
 * `DiagnoseOutcomeDisplay` (latest row only) and `DiagnoseHistoryList` (every
 * row) both need this exact mapping, so it lives here once instead of
 * drifting between two switch statements (this repo's jscpd duplication
 * gate also flags a copy-pasted switch like this).
 */
export function describeDiagnoseOutcome(d: DiagnoseOutcomeCopyInput): DiagnoseOutcomeCopy {
  switch (d.outcomeKind) {
    case "nudged":
      return { icon: "🟢", text: "nudged the session" };
    case "skipped_safety_gate":
      return { icon: "🟡", text: safetyGateReasonCopy(d.safetyGateReason) };
    case "bug_filed":
      return { icon: "🐛", text: "filed a bug instead of nudging" };
    case "inconclusive_note_filed":
      return { icon: "⚪", text: "inconclusive" };
    case "dispatch_failed":
      return {
        icon: "⚠",
        text: `Couldn't start diagnosis${d.failureReason ? ` — ${d.failureReason}` : ""}. Try again.`,
      };
    default:
      return { icon: "•", text: "diagnosis completed" };
  }
}
