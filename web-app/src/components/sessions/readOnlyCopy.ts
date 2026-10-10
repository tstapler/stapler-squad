// Copy for the hidden-session read-only view (plan Story 5.3, design/ux.md Surface 12).

export const READ_ONLY_BANNER_PRIMARY = "Background session - read-only";

// One constant (ADV-N36): PR 5u added the unary guards (WriteToSession, the UpdateSession
// text fields, the steer branch, Restart and SwitchWorkspace), so the view may now say
// that nothing can be typed. Until PR 5u the copy read "Terminal input is disabled in
// this view." because only the stream was guarded.
export const READ_ONLY_SECONDARY = "You can read output but not type.";

export const READ_ONLY_SECONDARY_WITH_REPLY =
  "You can read output. Reply to Claude's question below; nothing else can be typed.";

export function readOnlySecondaryText(replyCardPresent: boolean): string {
  return replyCardPresent ? READ_ONLY_SECONDARY_WITH_REPLY : READ_ONLY_SECONDARY;
}

export const READ_ONLY_REFUSAL_TOAST = "This session is read-only";

/** True for the server's guard refusal ("this session is a background session and is read-only"). */
export function isReadOnlyRefusal(message: string | null | undefined): boolean {
  return !!message && /background session.*read-only/i.test(message);
}
