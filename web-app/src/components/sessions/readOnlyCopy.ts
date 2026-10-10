// Copy for the hidden-session read-only view (plan Story 5.3, design/ux.md Surface 12).

export const READ_ONLY_BANNER_PRIMARY = "Background session - read-only";

// PR 5 ships the stream guards only; WriteToSession, the UpdateSession text fields and the
// backlog steer are guarded by PR 5u. Until then the copy claims only what the view enforces.
// PR 5u switches this constant to READ_ONLY_SECONDARY_FULL (and its test).
export const READ_ONLY_SECONDARY_STREAM_ONLY = "Terminal input is disabled in this view.";
export const READ_ONLY_SECONDARY_FULL = "You can read output but not type.";
export const READ_ONLY_SECONDARY = READ_ONLY_SECONDARY_STREAM_ONLY;

export const READ_ONLY_SECONDARY_WITH_REPLY =
  "You can read output. Reply to Claude's question below; nothing else can be typed.";

export function readOnlySecondaryText(replyCardPresent: boolean): string {
  return replyCardPresent ? READ_ONLY_SECONDARY_WITH_REPLY : READ_ONLY_SECONDARY;
}
