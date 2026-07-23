// Shared status display helpers for backlog items.
// CSS class mappings are intentionally kept per-component (vanilla-extract
// generates scoped class names), but labels and the fallback formatter are shared.

export const STATUS_LABELS: Record<string, string> = {
  idea: "Idea",
  refining: "Refining",
  ready: "Ready",
  queued: "Queued",
  in_progress: "In Progress",
  review: "Review",
  done: "Done",
  archived: "Archived",
};

/** Human-readable label for any status string, including unknown server-defined ones. */
export const getStatusLabel = (s: string): string =>
  STATUS_LABELS[s] ?? s.replace(/_/g, " ");
