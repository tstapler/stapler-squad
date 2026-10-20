import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ActionOutcome } from "@/gen/session/v1/notification_action_pb";
import { AnalyticsContext } from "@/lib/contexts/AnalyticsContext";
import type { NotificationHistoryItem } from "@/lib/types/notification";
import { NotificationPanel } from "./NotificationPanel";
import { makeApprovalDescriptors, makeNotification } from "./__fixtures__/notificationFixtures";
import { invokeMock, respondWith } from "./__fixtures__/actionRowHarness";

jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({ useFeatureFlag: () => false }));
jest.mock("@/lib/api/notificationActionApi", () => ({ invokeNotificationAction: jest.fn() }));
jest.mock("@/lib/hooks/useFeatureFlagValue", () => ({
  ...jest.requireActual("@/lib/hooks/useFeatureFlagValue"),
  useFeatureFlagValue: () => true,
}));
// jsdom has no layout: window the first 20 rows like a real viewport would.
jest.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () => Array.from({ length: Math.min(count, 20) }, (_, index) => ({ index, key: index, start: index * 100, size: 100 })),
    getTotalSize: () => count * 100,
    measureElement: () => {},
    scrollToIndex: jest.fn(),
  }),
}));
jest.mock("@/lib/hooks/useBackgroundSessions", () => ({
  useBackgroundSessions: () => ({ sessions: [], departed: [], loading: false, failed: false, lastUpdatedAt: null, refresh: () => {} }),
}));
jest.mock("@/lib/hooks/useNotificationConnectivity", () => ({
  useNotificationConnectivity: () => ({ state: "connected", isOffline: false }),
}));
jest.mock("@connectrpc/connect", () => ({ createClient: jest.fn(() => ({})) }));
jest.mock("@connectrpc/connect-web", () => ({ createConnectTransport: jest.fn(() => ({})) }));
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost:8543" }));

const refresh = jest.fn();
const acknowledge = jest.fn();
let history: NotificationHistoryItem[] = [];
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({
    notificationHistory: history,
    isPanelOpen: true,
    togglePanel: jest.fn(),
    markAsRead: jest.fn(),
    acknowledgeNotification: acknowledge,
    clearHistoryByIds: jest.fn(),
    showActionToast: jest.fn(),
    getUnreadCount: () => history.length,
    historyLoading: false,
    historyHasMore: false,
    historyError: null,
    historyLastUpdatedAt: 1,
    loadMoreHistory: jest.fn(),
    refreshHistory: refresh,
    quietMode: false,
    setQuietMode: jest.fn(),
  }),
}));

const actionable = (n: number) =>
  Array.from({ length: n }, (_, i) =>
    makeNotification({
      id: `n${i}`,
      sessionId: `s${i}`,
      notificationType: "approval_needed",
      isPendingDecision: true,
      metadata: { approval_id: `a${i}`, tool_name: "Bash", tool_input_command: `echo ${i}` },
      actions: makeApprovalDescriptors(`a${i}`),
    }),
  );

beforeEach(() => {
  history = actionable(40);
  refresh.mockClear();
  acknowledge.mockClear();
  invokeMock.mockReset();
});

describe("NotificationPanel inline actions", () => {
  it("commits the first action row in the opening render, from cached history, and posts the open time", () => {
    const track = jest.fn();
    render(
      <AnalyticsContext.Provider value={{ provider: {} as never, track }}>
        <NotificationPanel />
      </AnalyticsContext.Provider>,
    );
    expect(screen.queryByTestId("tray-skeleton")).toBeNull();
    expect(screen.getAllByTestId("action-row").length).toBeGreaterThan(0);
    expect(track).toHaveBeenCalledWith(expect.objectContaining({ name: "notification_pane_open_ms" }));
  });

  it("acknowledges the record and keeps the actionable item undismissable after Approve", async () => {
    respondWith(ActionOutcome.APPLIED);
    history = actionable(1);
    render(<NotificationPanel />);
    const row = screen.getByTestId("action-row");
    await userEvent.click(within(row).getByTestId("action-approve"));
    await within(row).findByText("Approved");
    expect(acknowledge).toHaveBeenCalledWith(["n0"]);
    expect(screen.queryByLabelText("Remove notification")).toBeNull();
  });

  it("refetches the history page once when INVALID is refreshed", async () => {
    respondWith(ActionOutcome.INVALID);
    history = actionable(1);
    render(<NotificationPanel />);
    await userEvent.click(screen.getByTestId("action-approve"));
    await userEvent.click(await screen.findByTestId("action-refresh"));
    expect(refresh).toHaveBeenCalledTimes(1);
  });
});
