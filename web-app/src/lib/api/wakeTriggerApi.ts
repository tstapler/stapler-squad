import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { SessionService } from "@/gen/session/v1/session_pb";
import {
  CancelWakeTriggerRequestSchema,
  CreateOperatorReminderRequestSchema,
  ListPeerHelpRequestsRequestSchema,
  ListWakeTriggersRequestSchema,
  WakeTriggerService,
  type CancelWakeTriggerResponse,
  type CreateOperatorReminderResponse,
  type ListPeerHelpRequestsResponse,
  type ListWakeTriggersResponse,
} from "@/gen/session/v1/wake_trigger_pb";

const client = () => createClient(WakeTriggerService, getConnectTransport());

export function listWakeTriggers(signal?: AbortSignal): Promise<ListWakeTriggersResponse> {
  return client().listWakeTriggers(create(ListWakeTriggersRequestSchema, {}), { signal });
}

export function listPeerHelpRequests(signal?: AbortSignal): Promise<ListPeerHelpRequestsResponse> {
  return client().listPeerHelpRequests(create(ListPeerHelpRequestsRequestSchema, {}), { signal });
}

// One-shot mutations from an explicit click: aborting on unmount would leave their outcome unknown.
export function cancelWakeTrigger(id: string): Promise<CancelWakeTriggerResponse> {
  // abort-signal-exempt
  return client().cancelWakeTrigger(create(CancelWakeTriggerRequestSchema, { id }));
}

export function createOperatorReminder(init: {
  sessionUuid: string;
  fireAt: Date;
  label: string;
}): Promise<CreateOperatorReminderResponse> {
  // abort-signal-exempt
  return client().createOperatorReminder(
    create(CreateOperatorReminderRequestSchema, {
      sessionUuid: init.sessionUuid,
      fireAt: timestampFromDate(init.fireAt),
      label: init.label,
    }),
  );
}

/** The kill-switch flag (ADR-008). */
export const WAKE_TRIGGERS_FLAG = "agent_wake_triggers";

/**
 * Turns agent_wake_triggers back on globally. The server audits the flip and may refuse it
 * (for example from the remote listener); the caller confirms by re-reading ListWakeTriggers.
 */
export async function resumeScheduledPrompts(): Promise<void> {
  // abort-signal-exempt
  await createClient(SessionService, getConnectTransport()).updateFeatureFlag({ name: WAKE_TRIGGERS_FLAG, enabled: true });
}
