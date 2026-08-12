export type ImportRowStatus =
  | "discovered"
  | "committed_pending_kill"
  | "imported"
  | "reverted"
  | "kill_failed";

export type ImportRowEvent =
  | { type: "commit_succeeded" }
  | { type: "commit_failed" }
  | { type: "kill_confirmed" }
  | { type: "kill_failed" }
  | { type: "kill_cancelled" }
  | { type: "cancel_failed" };

const TERMINAL_STATUSES: ReadonlySet<ImportRowStatus> = new Set([
  "imported",
  "reverted",
]);

export function isTerminalImportRowStatus(status: ImportRowStatus): boolean {
  return TERMINAL_STATUSES.has(status);
}

export function nextImportRowStatus(
  current: ImportRowStatus,
  event: ImportRowEvent
): ImportRowStatus {
  if (isTerminalImportRowStatus(current)) {
    return current;
  }
  switch (event.type) {
    case "commit_succeeded":
      return "committed_pending_kill";
    case "commit_failed":
      return "discovered";
    case "kill_confirmed":
      return "imported";
    case "kill_failed":
      return "kill_failed";
    case "kill_cancelled":
      return "reverted";
    case "cancel_failed":
      return "committed_pending_kill";
    default:
      return current;
  }
}
