import { create } from "@bufbuild/protobuf";
import type {
  ActionOutcome} from "@/gen/session/v1/notification_action_pb";
import {
  ActionKind,
  ActionOptionSchema,
  ActionTargetKey,
  ActionTier,
  InvokeNotificationActionResponseSchema,
  NotificationActionDescriptorSchema,
  type NotificationActionDescriptor,
} from "@/gen/session/v1/notification_action_pb";
import type { NotificationHistoryItem } from "@/lib/types/notification";
import type { PlainGuidanceRequest } from "@/lib/api/guidanceApi";
import type { BackgroundRow, BackgroundOutcome, HiddenSessionSummary } from "@/lib/utils/backgroundActivity";

export function makeNotification(overrides: Partial<NotificationHistoryItem> = {}): NotificationHistoryItem {
  return {
    id: overrides.id ?? "notif-1",
    sessionId: "sess-1",
    sessionName: "my-session",
    message: "Something happened",
    timestamp: Date.now(),
    isRead: false,
    notificationType: "info",
    ...overrides,
  };
}

export function makeHiddenSession(over: Partial<HiddenSessionSummary> = {}): HiddenSessionSummary {
  const id = over.id ?? "review:h1";
  return { id, title: id, state: "stopped", updatedAtMs: Date.now() - 3 * 60_000, backgroundOutcome: "unknown", ...over };
}

/** The live-shaped list: 10 stopped `review:*` sessions and 1 stopped `diagnose:*`, newest first. */
export function makeElevenStopped(reviewOutcome: BackgroundOutcome = "completed", now = Date.now()): HiddenSessionSummary[] {
  const reviews = Array.from({ length: 10 }, (_, i) =>
    makeHiddenSession({ id: `review:r${i}`, backgroundOutcome: reviewOutcome, updatedAtMs: now - (3 + i) * 60_000 }),
  );
  return [...reviews, makeHiddenSession({ id: "diagnose:d1", backgroundOutcome: "unknown", updatedAtMs: now - 60 * 60_000 })];
}

export function makeBackgroundRow(over: Partial<BackgroundRow> = {}): BackgroundRow {
  return {
    key: "bg:review:h1",
    sessionId: "review:h1",
    sessionTitle: "review:h1",
    kind: "failure",
    statusLabel: "FAILED",
    title: "Review failed",
    message: "3 tests failing",
    timestampMs: Date.now() - 4 * 60_000,
    recordIds: ["n1"],
    primaryRecordId: "n1",
    sessionAvailable: true,
    pendingQuestion: false,
    ...over,
  };
}

/** A `useBackgroundSessions` state, as the NotificationPanel suite mocks it. */
export function makeHiddenState(over: Record<string, unknown> = {}) {
  return {
    sessions: [{ id: "review:h1", title: "review:h1", state: "running", updatedAtMs: Date.now() }],
    departed: [],
    loading: false,
    failed: false,
    lastUpdatedAt: Date.now(),
    refresh: () => {},
    ...over,
  };
}

/** `count` stopped hidden sessions `review:s0..`, the first `old` of them last active 30 h ago. */
export function makeStoppedSessions(count: number, old = 0, now = Date.now()): HiddenSessionSummary[] {
  return Array.from({ length: count }, (_, i) =>
    makeHiddenSession({ id: `review:s${i}`, updatedAtMs: i < old ? now - 30 * 3_600_000 - i : now - (3 + i) * 1000 }),
  );
}

type DescriptorInit = Partial<Omit<NotificationActionDescriptor, "$typeName" | "$unknown" | "options">> & {
  options?: Array<{ label: string; value?: string }>;
};

export function makeDescriptor(over: DescriptorInit = {}): NotificationActionDescriptor {
  const { options = [], ...rest } = over;
  return create(NotificationActionDescriptorSchema, {
    kind: ActionKind.OPEN,
    label: "Open",
    tier: ActionTier.A,
    available: true,
    ...rest,
    options: options.map((o) => create(ActionOptionSchema, o)),
  });
}

/** What the server emits for a pending approval: Approve, Deny, Open. */
export function makeApprovalDescriptors(id = "appr-1", over: DescriptorInit = {}): NotificationActionDescriptor[] {
  const target = { targetKeyType: ActionTargetKey.APPROVAL_ID, targetKey: id, ...over };
  return [
    makeDescriptor({ kind: ActionKind.APPROVE, label: "Approve", ...target }),
    makeDescriptor({ kind: ActionKind.DENY, label: "Deny", ...target }),
    makeDescriptor(),
  ];
}

export function makeDecisionDescriptors(itemId = "item-1"): NotificationActionDescriptor[] {
  return [
    makeDescriptor({
      kind: ActionKind.BACKLOG_DECISION,
      label: "Decide",
      targetKeyType: ActionTargetKey.ITEM_ID,
      targetKey: itemId,
      options: [
        { value: "accept", label: "Mark done" },
        { value: "rework", label: "Send back" },
        { value: "merge", label: "Merge" },
      ],
    }),
    makeDescriptor(),
  ];
}

export function makeQuestionDescriptors(over: DescriptorInit = {}): NotificationActionDescriptor[] {
  return [
    makeDescriptor({
      kind: ActionKind.ANSWER_QUESTION,
      label: "Answer",
      tier: ActionTier.B,
      targetKeyType: ActionTargetKey.QUESTION_ID,
      targetKey: "q1",
      sessionId: "sess-1",
      options: [{ label: "Redis" }, { label: "Memcached" }],
      ...over,
    }),
    makeDescriptor(),
  ];
}

export function makeActionResponse(outcome: ActionOutcome, over: { staleReason?: string; retryAfterSeconds?: number; detail?: string } = {}) {
  return create(InvokeNotificationActionResponseSchema, { outcome, ...over });
}

/** A durable question row as GetGuidanceRequest returns it (hook-mirrored unless `over` says otherwise). */
export function makeGuidanceRow(over: Partial<PlainGuidanceRequest> = {}): PlainGuidanceRequest {
  return {
    id: "q1",
    scope: "session",
    sessionUuid: "sess-1",
    questionText: "Which cache?",
    questionType: "multiple-choice",
    options: [],
    answer: "",
    status: "pending",
    createdAt: undefined,
    answeredAt: undefined,
    source: "ask_user_question_hook",
    askerSessionUuid: "sess-1",
    questions: [{ question: "Which cache?", header: "Cache", multiSelect: false, options: [{ label: "Redis", description: "" }, { label: "Memcached", description: "" }] }],
    deliveryRoute: "live_keystroke",
    deliveryState: "",
    unacknowledged: false,
    cancelReason: "",
    sessionEnded: false,
    sessionArchived: false,
    ...over,
  };
}

/** The question-card descriptor of a durable-record route (an agent-created request): Tier A, text allowed. */
export function makeGuidanceDescriptors(over: DescriptorInit = {}): NotificationActionDescriptor[] {
  return [
    makeDescriptor({ kind: ActionKind.ANSWER_GUIDANCE, label: "Answer", targetKeyType: ActionTargetKey.GUIDANCE_REQUEST_ID, targetKey: "g1", ...over }),
    makeDescriptor(),
  ];
}
