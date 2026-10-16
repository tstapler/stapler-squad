import type { UpNextTab } from "./upNextTab";

export const TAB_STATS_STORAGE_KEY = "up-next-tab-stats";
const LEFT_PRS_WINDOW_MS = 5000;

export interface TabStats {
  visits: number;
  landed: Record<UpNextTab, number>;
  leftPrsWithin5s: number;
  firstPrCardMs: number | null;
  /** "Open session" clicks from a PR card: a success signal alongside DELIVERED nudges. */
  openSessionClicks: number;
}

export type TabEvent =
  | { type: "visit"; landedOn: UpNextTab }
  | { type: "click"; from: UpNextTab; to: UpNextTab; msSinceVisit: number; firstClickOfVisit: boolean }
  | { type: "firstPrCard"; ms: number }
  | { type: "openSession" };

export function emptyTabStats(): TabStats {
  return {
    visits: 0,
    landed: { prs: 0, stuck: 0, worktrees: 0, queue: 0 },
    leftPrsWithin5s: 0,
    firstPrCardMs: null,
    openSessionClicks: 0,
  };
}

/** Pure reducer; counters only, never PR content. */
export function recordTabEvent(stats: TabStats, event: TabEvent): TabStats {
  switch (event.type) {
    case "visit":
      return {
        ...stats,
        visits: stats.visits + 1,
        landed: { ...stats.landed, [event.landedOn]: stats.landed[event.landedOn] + 1 },
      };
    case "click": {
      const leftPrsQuickly =
        event.firstClickOfVisit &&
        event.from === "prs" &&
        event.to !== "prs" &&
        event.msSinceVisit <= LEFT_PRS_WINDOW_MS;
      return leftPrsQuickly ? { ...stats, leftPrsWithin5s: stats.leftPrsWithin5s + 1 } : stats;
    }
    case "firstPrCard":
      return { ...stats, firstPrCardMs: event.ms };
    case "openSession":
      return { ...stats, openSessionClicks: stats.openSessionClicks + 1 };
  }
}

export function readStats(): TabStats {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(TAB_STATS_STORAGE_KEY) ?? "null");
    if (parsed && typeof parsed === "object") {
      const base = emptyTabStats();
      return {
        visits: Number(parsed.visits) || 0,
        landed: { ...base.landed, ...(parsed.landed ?? {}) },
        leftPrsWithin5s: Number(parsed.leftPrsWithin5s) || 0,
        firstPrCardMs: typeof parsed.firstPrCardMs === "number" ? parsed.firstPrCardMs : null,
        openSessionClicks: Number(parsed.openSessionClicks) || 0,
      };
    }
  } catch {
    // Missing, malformed or blocked storage: start from zero.
  }
  return emptyTabStats();
}

export function writeStats(stats: TabStats): void {
  try {
    window.localStorage.setItem(TAB_STATS_STORAGE_KEY, JSON.stringify(stats));
  } catch {
    // Metrics are best-effort.
  }
}

/** Read-modify-write convenience for call sites outside the pure reducer. */
export function emitTabEvent(event: TabEvent): void {
  writeStats(recordTabEvent(readStats(), event));
}
