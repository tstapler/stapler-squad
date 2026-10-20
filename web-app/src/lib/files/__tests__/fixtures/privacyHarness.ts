import type { AnalyticsEvent } from "@/lib/analytics/types";
import { FILES_EVENTS, STAMP_KEYS, type FilesEventName } from "@/lib/files/analytics/events";

/** Values that must never appear anywhere in a tracked event (ADR-035 canary protocol). */
export const CANARIES = {
  path: "secret/dir/Canary.go",
  name: "Canary.go",
  query: "CANARYQUERY",
  glob: "**/Canary*",
  draft: "CANARYDRAFT",
  branch: "canary-branch",
} as const;

export const LABEL_VALUE_SHAPE = /^[a-z0-9_+-]{1,24}$/;
export const MAX_LABELS = 8;

/** Lowercased JSON of the whole event, so a canary hides in no field or letter case. */
export function canaryLeaks(events: readonly AnalyticsEvent[]): string[] {
  const json = JSON.stringify(events).toLowerCase();
  return Object.values(CANARIES).filter((c) => json.includes(c.toLowerCase()));
}

/** Problems with one event's labels versus the registry; an empty list means it is clean. */
export function labelProblems(event: AnalyticsEvent): string[] {
  const def = FILES_EVENTS[event.name as FilesEventName] as { labels: Record<string, readonly string[]> } | undefined;
  if (!def) return [`unknown event ${event.name}`];
  const labels = event.labels ?? {};
  const problems: string[] = [];
  if (Object.keys(labels).length > MAX_LABELS) problems.push("more than 8 labels");
  for (const [key, value] of Object.entries(labels)) {
    if ((STAMP_KEYS as readonly string[]).includes(key)) {
      if (!LABEL_VALUE_SHAPE.test(value)) problems.push(`${key} shape`);
      continue;
    }
    if (!(key in def.labels)) problems.push(`${key} is not a registry key`);
    else if (!def.labels[key].includes(value)) problems.push(`${key}=${value} is outside its set`);
    if (!LABEL_VALUE_SHAPE.test(value)) problems.push(`${key} shape`);
  }
  return problems;
}

/** One valid label set per event: the first member of every label's set. */
export function fixtureLabels(name: FilesEventName): Record<string, string> {
  const def = FILES_EVENTS[name] as { labels: Record<string, readonly string[]> };
  return Object.fromEntries(Object.entries(def.labels).map(([k, set]) => [k, set[0]]));
}
