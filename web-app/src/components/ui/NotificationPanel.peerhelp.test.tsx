import { act, screen } from "@testing-library/react";
import { ActionKind, ActionTargetKey } from "@/gen/session/v1/notification_action_pb";
import { setTimingForTests } from "@/lib/hooks/interactionTiming";
import { makeDescriptor, makeNotification } from "./__fixtures__/notificationFixtures";
import { pendingApprovals, renderPanel, rowKeys } from "./__fixtures__/panelHarness";
import { panelState } from "./__fixtures__/panelMocks";

jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({ useFeatureFlag: () => false }));
jest.mock("@/lib/api/notificationActionApi", () => ({ invokeNotificationAction: jest.fn() }));
jest.mock("@/lib/hooks/useFeatureFlagValue", () => ({
  ...jest.requireActual("@/lib/hooks/useFeatureFlagValue"),
  useFeatureFlagValue: () => true,
}));
jest.mock("@tanstack/react-virtual", () => require("./__fixtures__/panelMocks").virtualizerModule());
jest.mock("@/lib/contexts/NotificationContext", () => require("./__fixtures__/panelMocks").notificationsModule());
jest.mock("@/lib/hooks/useBackgroundSessions", () => ({
  useBackgroundSessions: () => ({ sessions: [], departed: [], loading: false, failed: false, lastUpdatedAt: null, refresh: () => {} }),
}));
jest.mock("@/lib/hooks/useNotificationConnectivity", () => ({
  useNotificationConnectivity: () => ({ state: "connected", isOffline: false }),
}));
jest.mock("@/lib/hooks/useScheduledPrompts", () => ({
  useScheduledPrompts: () => ({ triggers: [], peers: [], enabled: false, loading: false, failed: false, lastUpdatedAt: 1, refresh: jest.fn() }),
}));
jest.mock("@connectrpc/connect", () => ({ createClient: jest.fn(() => ({})) }));
jest.mock("@connectrpc/connect-web", () => ({ createConnectTransport: jest.fn(() => ({})) }));
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost:8543" }));

const GRACE = 750;

/** What the server raises 10 minutes after a peer request stalls: INPUT_REQUIRED with its three actions. */
const escalation = (id: string) =>
  makeNotification({
    id,
    sessionId: "asker-uuid",
    sessionName: "review:pr-1042",
    timestamp: Date.now(),
    notificationType: "question",
    isPendingDecision: true,
    title: "Help request stalled",
    message: "review:pr-1042 asked diagnose:abc: no reply in 10m. Which approach?",
    metadata: { guidance_request_id: "g1", peer_title: "diagnose:abc" },
    actions: [
      makeDescriptor({ kind: ActionKind.ANSWER_GUIDANCE, label: "Answer for diagnose:abc", targetKeyType: ActionTargetKey.GUIDANCE_REQUEST_ID, targetKey: "g1" }),
      makeDescriptor({ kind: ActionKind.CANCEL_PEER_HELP, label: "Cancel request", targetKeyType: ActionTargetKey.GUIDANCE_REQUEST_ID, targetKey: "g1", requiresConfirm: true }),
      makeDescriptor(),
    ],
  });

beforeEach(() => {
  jest.useFakeTimers();
  setTimingForTests({ settleMs: 0, commitDelayMs: 0 });
  panelState.history = pendingApprovals(3);
});
afterEach(() => jest.useRealTimers());

const needsYou = (container: HTMLElement) => rowKeys(container).filter((k) => k?.startsWith("n") || k?.startsWith("esc"));

describe("Story 4.11d: peer-help escalation presentation", () => {
  it("PeerHelpEscalation_should_raiseBadgeAnnounceOnceAndKeepFocus_And_waitBehindNNew_When_cardEngaged", () => {
    const { container, refreshPanel, announce } = renderPanel();
    const approve = screen.getAllByTestId("action-approve")[0];
    act(() => approve.focus());
    const before = needsYou(container);

    panelState.history = [escalation("esc1"), ...panelState.history];
    refreshPanel();
    refreshPanel();

    expect(announce.mock.calls.filter(([text]) => text === "1 item needs a decision")).toHaveLength(1);
    expect(document.activeElement).toBe(approve);
    expect(needsYou(container)).toEqual(before);
    expect(screen.getByTestId("tray-new-pill")).toHaveTextContent("1 new");

    act(() => (document.activeElement as HTMLElement).blur());
    act(() => void jest.advanceTimersByTime(GRACE));
    expect(needsYou(container)).toEqual(["esc1", ...before]);
    expect(screen.getByRole("button", { name: "Cancel request" })).toBeInTheDocument();
  });

  it("ux_ac80_should_announceOnlyEscalationsRaisedAfterOpening", () => {
    panelState.history = [escalation("old"), ...pendingApprovals(1)];
    panelState.history[0] = { ...panelState.history[0], timestamp: Date.now() - 3_600_000 };
    const { refreshPanel, announce } = renderPanel();
    refreshPanel();
    expect(announce.mock.calls.filter(([text]) => String(text).includes("needs a decision"))).toHaveLength(0);

    panelState.history = [escalation("fresh"), ...panelState.history];
    refreshPanel();
    expect(announce.mock.calls.filter(([text]) => text === "1 item needs a decision")).toHaveLength(1);
  });
});
