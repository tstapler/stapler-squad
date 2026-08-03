export type EndReasonSeverity = "none" | "warning" | "error";

export interface FormattedEndReason {
  label: string;
  severity: EndReasonSeverity;
}

/**
 * Maps ItemSession.end_reason to a chip label + severity. "" and "shutdown"
 * are the documented clean-exit cases and render no chip. Every other value —
 * including one this switch doesn't recognize yet — renders a visible chip:
 * silently treating an unrecognized future end_reason as "none" would make a
 * genuine failure indistinguishable from clean success, which is exactly the
 * gap this chip exists to close.
 */
export function formatEndReason(reason: string): FormattedEndReason {
  switch (reason) {
    case "":
    case "shutdown":
      return { label: "", severity: "none" };
    case "timeout":
      return { label: "Headless call timed out", severity: "warning" };
    case "process_error":
      return { label: "Headless call failed (process error)", severity: "warning" };
    case "claude_not_found":
      return { label: "Headless call failed — claude CLI not found", severity: "error" };
    case "other":
      return { label: "Headless call failed (unclassified)", severity: "error" };
    default:
      return { label: `Unrecognized end reason: ${reason}`, severity: "warning" };
  }
}
