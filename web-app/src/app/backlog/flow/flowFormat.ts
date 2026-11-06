import type { FlowWindow } from "@/lib/hooks/useBacklogFlow";

const pad = (n: number) => String(n).padStart(2, "0");

/** Local wall-clock time, `14:02` or `14:02:11`. */
export function formatClock(date: Date, withSeconds = false): string {
  const hm = `${pad(date.getHours())}:${pad(date.getMinutes())}`;
  return withSeconds ? `${hm}:${pad(date.getSeconds())}` : hm;
}

/** `12s ago`, `3m ago`, `2h ago`, `5d ago`; a clock slightly ahead of the server reads `0s ago`. */
export function formatAge(ms: number): string {
  const seconds = Math.max(0, Math.floor(ms / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

export function windowLabel(window: FlowWindow): string {
  return window === "all" ? "All time" : `${window}d`;
}

/** The next wider window, or null when there is none (all time). */
export function widerWindow(window: FlowWindow): FlowWindow | null {
  if (window === 7) return 30;
  if (window === 30) return 90;
  if (window === 90) return "all";
  return null;
}
