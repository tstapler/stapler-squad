import { ActionKind, type ActionOption, type NotificationActionDescriptor } from "@/gen/session/v1/notification_action_pb";
import { COPY, PENDING_VERB, type HoldKey } from "@/lib/copy/notificationCopy";
import type { ActionSpec } from "@/lib/hooks/useInvokeNotificationAction";

/** A descriptor, plus the option the operator picked from it. */
export interface Pick {
  descriptor: NotificationActionDescriptor;
  option?: ActionOption;
}

export const OPTION_KINDS = new Set([ActionKind.ANSWER_QUESTION, ActionKind.ANSWER_GUIDANCE, ActionKind.BACKLOG_DECISION]);
export const CANCEL_KINDS = new Set([ActionKind.CANCEL_WAKE, ActionKind.CANCEL_PEER_HELP]);

/** Kinds this client can render; anything else degrades to Open only. */
export const KNOWN_KINDS = new Set([ActionKind.APPROVE, ActionKind.DENY, ...OPTION_KINDS, ...CANCEL_KINDS]);

const MERGE_CHOICE = "merge";

export const kindSlug = (k: ActionKind) => ActionKind[k].toLowerCase();

export const isMerge = (p: Pick) => p.option?.value === MERGE_CHOICE;

/** Cancel actions, merge and any `requires_confirm` descriptor run an inline confirm first. */
export function needsConfirm(p: Pick): boolean {
  return CANCEL_KINDS.has(p.descriptor.kind) || p.descriptor.requiresConfirm || isMerge(p);
}

export function buildSpec(p: Pick, extra: { reason?: string; confirm?: boolean } = {}): ActionSpec {
  const { descriptor: d, option } = p;
  const params: NonNullable<ActionSpec["params"]> = { ...extra };
  if (d.kind === ActionKind.BACKLOG_DECISION) params.choice = option?.value;
  else if (OPTION_KINDS.has(d.kind)) params.answer = option?.label;
  return { kind: d.kind, targetKeyType: d.targetKeyType, targetKey: d.targetKey, sessionId: d.sessionId, params };
}

export function unavailableReason(d: NotificationActionDescriptor): string {
  return d.unavailableReason === "desktop_only" ? COPY.desktopOnly : COPY.forbidden;
}

export function pendingVerb(kind: ActionKind): string {
  if (kind === ActionKind.APPROVE) return PENDING_VERB.approve;
  if (kind === ActionKind.DENY) return PENDING_VERB.deny;
  return PENDING_VERB.other;
}

/** Approve, Send back (`rework`) and Mark done (`accept`) wait behind Undo; nothing else does. */
export function holdKeyOf(spec: ActionSpec): HoldKey | null {
  if (spec.kind === ActionKind.APPROVE) return "approve";
  if (spec.kind === ActionKind.BACKLOG_DECISION && (spec.params?.choice === "rework" || spec.params?.choice === "accept")) return spec.params.choice;
  return null;
}
