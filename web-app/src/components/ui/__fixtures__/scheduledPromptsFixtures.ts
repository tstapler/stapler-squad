import { create } from "@bufbuild/protobuf";
import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import { PeerHelpInfoSchema, WakeTriggerInfoSchema, type PeerHelpInfo, type WakeTriggerInfo } from "@/gen/session/v1/wake_trigger_pb";
import type { ScheduledPromptsState } from "@/lib/hooks/useScheduledPrompts";

/** A fixed "now" (13:10 UTC) so ages and clock times are stable. */
export const NOW = Date.UTC(2026, 9, 19, 13, 10, 0);
export const MIN = 60_000;

let seq = 0;

export function makeTrigger(over: Partial<Omit<WakeTriggerInfo, "$typeName" | "nextFireAt" | "createdAt" | "deliveredAt" | "endedAt">> & {
  nextFireAt?: number;
  createdAt?: number;
} = {}): WakeTriggerInfo {
  seq += 1;
  const { nextFireAt, createdAt, ...rest } = over;
  return create(WakeTriggerInfoSchema, {
    id: `w${seq}`,
    ownerSessionUuid: `owner-${seq}`,
    ownerTitle: `diagnose:abc${seq}`,
    ownerExists: true,
    triggerType: "at",
    actionKind: "steer_text",
    status: "pending",
    message: "check on the build",
    createdAt: timestampFromMs(createdAt ?? NOW - 12 * MIN),
    nextFireAt: nextFireAt === undefined ? timestampFromMs(NOW + 30 * MIN) : timestampFromMs(nextFireAt),
    ...rest,
  });
}

export function makePeer(over: Partial<Omit<PeerHelpInfo, "$typeName" | "createdAt" | "answeredAt">> & { createdAt?: number } = {}): PeerHelpInfo {
  seq += 1;
  const { createdAt, ...rest } = over;
  return create(PeerHelpInfoSchema, {
    id: `p${seq}`,
    askerSessionUuid: "review-uuid",
    askerTitle: "review:pr-1042",
    answererSessionUuid: "diagnose-uuid",
    answererTitle: "diagnose:abc",
    question: "Which approach for the retry logic?",
    state: "waiting",
    createdAt: timestampFromMs(createdAt ?? NOW - 3 * MIN),
    ...rest,
  });
}

export function makeState(over: Partial<ScheduledPromptsState> = {}): ScheduledPromptsState {
  return {
    triggers: [],
    peers: [],
    enabled: true,
    loading: false,
    failed: false,
    lastUpdatedAt: NOW,
    refresh: async () => true,
    ...over,
  };
}
