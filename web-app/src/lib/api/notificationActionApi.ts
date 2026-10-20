import { create } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { SessionService } from "@/gen/session/v1/session_pb";
import {
  InvokeNotificationActionRequestSchema,
  NotificationActionParamsSchema,
  type ActionKind,
  type ActionTargetKey,
  type InvokeNotificationActionResponse,
} from "@/gen/session/v1/notification_action_pb";

/** Labels the audit channel (web_local / web_remote); the server treats any other caller as unknown. */
export const ACTION_CLIENT_HEADER = "X-Stapler-Client";
export const ACTION_CLIENT_WEB_UI = "web-ui";

export interface InvokeInit {
  actionKind: ActionKind;
  targetKeyType: ActionTargetKey;
  targetKey: string;
  sessionId: string;
  params?: { answer?: string; choice?: string; reason?: string; confirm?: boolean };
  idempotencyKey: string;
}

export function invokeNotificationAction(init: InvokeInit): Promise<InvokeNotificationActionResponse> {
  const client = createClient(SessionService, getConnectTransport());
  // One-shot mutation from an explicit click; aborting on unmount would leave its outcome unknown.
  // abort-signal-exempt
  return client.invokeNotificationAction(create(InvokeNotificationActionRequestSchema, {
      ...init,
      params: init.params && create(NotificationActionParamsSchema, init.params),
    }), {
    headers: { [ACTION_CLIENT_HEADER]: ACTION_CLIENT_WEB_UI },
  });
}
