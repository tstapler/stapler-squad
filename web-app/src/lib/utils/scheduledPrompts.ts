import { timestampMs, type Timestamp } from "@bufbuild/protobuf/wkt";
import type { WakeTriggerInfo } from "@/gen/session/v1/wake_trigger_pb";
import { SCHEDULED_COPY } from "@/lib/copy/scheduledPromptsCopy";

/** Milliseconds since the epoch of a protobuf timestamp, 0 when unset. */
export const tsMs = (ts: Timestamp | undefined): number => (ts ? timestampMs(ts) : 0);

/** "3m", "2h", "4d": the coarsest unit that is not zero; "under 1m" below a minute. */
export function ageText(ms: number): string {
  const mins = Math.floor(Math.max(0, ms) / 60_000);
  if (mins < 1) return "under 1m";
  if (mins < 60) return `${mins}m`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}

/** The plain-language "when" of a trigger: "prompts again at 13:40", "prompts again when PR #901 CI passes". */
export function conditionText(t: WakeTriggerInfo, clock: (ms: number) => string): string {
  if (t.triggerType === "at") return SCHEDULED_COPY.promptsAgainAt(clock(tsMs(t.nextFireAt)));
  const zone = t.cronTimezone ? ` (${t.cronTimezone})` : "";
  return SCHEDULED_COPY.promptsAgain(`${t.condition}${zone}`);
}

/**
 * Missed rows first (they are exceptions), then time-based rows by fire time, then event rows
 * newest first. Stable for equal keys so a poll never shuffles rows.
 */
export function sortTriggers(list: readonly WakeTriggerInfo[]): WakeTriggerInfo[] {
  const rank = (t: WakeTriggerInfo) => (t.status === "missed" ? 0 : t.nextFireAt ? 1 : 2);
  return [...list].sort((a, b) => {
    const byRank = rank(a) - rank(b);
    if (byRank !== 0) return byRank;
    if (rank(a) === 1) return tsMs(a.nextFireAt) - tsMs(b.nextFireAt);
    return tsMs(b.createdAt) - tsMs(a.createdAt);
  });
}
