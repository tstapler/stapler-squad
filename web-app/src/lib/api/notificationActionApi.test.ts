import { ActionKind, ActionTargetKey } from "@/gen/session/v1/notification_action_pb";
import { ACTION_CLIENT_HEADER, ACTION_CLIENT_WEB_UI, invokeNotificationAction } from "./notificationActionApi";

const rpc = jest.fn().mockResolvedValue({});
jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ invokeNotificationAction: rpc }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

describe("invokeNotificationAction", () => {
  it("labels the call as the web UI so the server audits the right channel", async () => {
    await invokeNotificationAction({
      actionKind: ActionKind.APPROVE,
      targetKeyType: ActionTargetKey.APPROVAL_ID,
      targetKey: "a1",
      sessionId: "",
      idempotencyKey: "k1",
      params: { reason: "r" },
    });
    const [request, options] = rpc.mock.calls[0];
    expect(options.headers).toEqual({ [ACTION_CLIENT_HEADER]: ACTION_CLIENT_WEB_UI });
    expect(ACTION_CLIENT_HEADER).toBe("X-Stapler-Client");
    expect(ACTION_CLIENT_WEB_UI).toBe("web-ui");
    expect(request.idempotencyKey).toBe("k1");
    expect(request.params.reason).toBe("r");
  });
});
