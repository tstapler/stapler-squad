import type { BoardColumnKey } from "./columns";

/**
 * Result of one attempted column move (drag-drop or MoveToMenu — both route through
 * attemptColumnMove so they produce identical outcomes). Distinguishes "your action was
 * rejected by the client" from "the server rejected it" from "the network failed" so the UI
 * can surface a visibly different toast/announcement for each (ux.md Surface 10).
 */
export type DragOutcome =
  | { type: "moved" }
  | { type: "rejected_illegal"; from: BoardColumnKey; to: BoardColumnKey }
  | { type: "rejected_by_server"; reason: string }
  | { type: "network_error" }
  | { type: "cancelled" };
