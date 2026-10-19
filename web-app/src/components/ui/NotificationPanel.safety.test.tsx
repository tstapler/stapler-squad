import { act, fireEvent, screen, within } from "@testing-library/react";
import { ActionOutcome } from "@/gen/session/v1/notification_action_pb";
import { invokeMock, respondWith } from "./__fixtures__/actionRowHarness";
import { setTimingForTests } from "@/lib/hooks/interactionTiming";
import { pendingApprovals, renderPanel, resolvedElsewhere, rowKeys } from "./__fixtures__/panelHarness";
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
jest.mock("@connectrpc/connect", () => ({ createClient: jest.fn(() => ({})) }));
jest.mock("@connectrpc/connect-web", () => ({ createConnectTransport: jest.fn(() => ({})) }));
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost:8543" }));

const GRACE = 750;
const advance = (ms: number) => act(() => void jest.advanceTimersByTime(ms));
const approveOf = (i: number) => screen.getAllByTestId("action-approve")[i];
/** The operator's own focus: dispatches focusin like Tab or a click would. */
const operatorFocus = (el: HTMLElement) => act(() => el.focus());

beforeEach(() => {
  jest.useFakeTimers();
  setTimingForTests({ settleMs: 0, commitDelayMs: 0 });
  panelState.history = pendingApprovals(3);
  panelState.acknowledge.mockClear();
});
afterEach(() => {
  jest.useRealTimers();
  setTimingForTests({ settleMs: 0, commitDelayMs: 0 });
});

describe("Story 2.10a: engaged-card freeze", () => {
  it("holds the Needs-you rows still while a card is focused, queues arrivals behind N new, and merges 750 ms after focus leaves", () => {
    const { container, refreshPanel, announce } = renderPanel();
    expect(rowKeys(container).filter((k) => k?.startsWith("n"))).toEqual(["n0", "n1", "n2"]);
    operatorFocus(approveOf(0));
    const [, n1, n2] = panelState.history;
    panelState.history = [...pendingApprovals(1, 9), panelState.history[0], resolvedElsewhere(n1), n2];
    refreshPanel();

    expect(rowKeys(container).filter((k) => k?.startsWith("n"))).toEqual(["n0", "n1", "n2"]);
    expect(within(container.querySelector(`[data-tray-row="n1"]`) as HTMLElement).getByText("✓ Approved")).toBeInTheDocument();
    expect(screen.getByTestId("tray-new-pill")).toHaveTextContent("1 new");

    act(() => (document.activeElement as HTMLElement).blur());
    advance(GRACE - 1);
    expect(rowKeys(container).filter((k) => k?.startsWith("n"))).toEqual(["n0", "n1", "n2"]);
    advance(1);
    expect(rowKeys(container).filter((k) => k?.startsWith("n"))).toEqual(["n9", "n0", "n2", "n1"]);
    expect(screen.queryByTestId("tray-new-pill")).toBeNull();
    expect(announce.mock.calls.filter(([text]) => text === "1 new item")).toHaveLength(1);
  });

  it("activating N new merges at once and moves focus to the first merged row", () => {
    const { container, refreshPanel } = renderPanel();
    operatorFocus(approveOf(0));
    panelState.history = [...pendingApprovals(1, 9), ...panelState.history];
    refreshPanel();
    act(() => screen.getByTestId("tray-new-pill").click());
    expect(rowKeys(container).filter((k) => k?.startsWith("n"))).toEqual(["n9", "n0", "n1", "n2"]);
    expect(document.activeElement).toBe(container.querySelector('[data-tray-row="n9"]'));
  });

  it("does not freeze when nothing is engaged", () => {
    const { container, refreshPanel } = renderPanel();
    panelState.history = [...pendingApprovals(1, 9), ...panelState.history];
    refreshPanel();
    expect(rowKeys(container).filter((k) => k?.startsWith("n"))).toEqual(["n9", "n0", "n1", "n2"]);
    expect(screen.queryByTestId("tray-new-pill")).toBeNull();
  });
});

const needsYou = (container: HTMLElement) => rowKeys(container).filter((k) => k?.startsWith("n"));

describe("Story 2.6a and 2.10: resolved-row lifecycle and focus advance", () => {
  beforeEach(() => invokeMock.mockReset());

  it("keeps an approved row in place, then removes it 750 ms after nothing is engaged and moves focus to the next header without engaging it", async () => {
    respondWith(ActionOutcome.APPLIED);
    const { container, refreshPanel } = renderPanel();
    operatorFocus(approveOf(0));
    await act(async () => approveOf(0).click());
    expect(within(container.querySelector('[data-tray-row="n0"]') as HTMLElement).getByText("Approved")).toBeInTheDocument();
    panelState.history = [resolvedElsewhere(panelState.history[0]), ...panelState.history.slice(1)];
    refreshPanel();
    expect(needsYou(container)).toEqual(["n0", "n1", "n2"]);

    advance(GRACE);
    expect(needsYou(container)).toEqual(["n1", "n2", "n0"]);
    expect(document.activeElement).toBe(container.querySelector('[data-tray-row="n1"]'));

    // Programmatic focus never engages: a poll adding a row inserts it at once, with no "N new".
    panelState.history = [...pendingApprovals(1, 9), ...panelState.history];
    refreshPanel();
    expect(needsYou(container)).toContain("n9");
    expect(screen.queryByTestId("tray-new-pill")).toBeNull();
  });

  it("keeps the row showing its result while the pointer is still over it", async () => {
    respondWith(ActionOutcome.APPLIED);
    const { container, refreshPanel } = renderPanel();
    const row = container.querySelector('[data-tray-row="n0"]') as HTMLElement;
    operatorFocus(approveOf(0));
    fireEvent.pointerEnter(row);
    await act(async () => approveOf(0).click());
    panelState.history = [resolvedElsewhere(panelState.history[0]), ...panelState.history.slice(1)];
    refreshPanel();
    advance(GRACE * 4);
    expect(needsYou(container)).toEqual(["n0", "n1", "n2"]);
    expect(screen.getByText("✓ Approved")).toBeInTheDocument();
  });

  it("announces the move once when a late acknowledgement retires the row that held focus (D6)", () => {
    const { container, refreshPanel, announce } = renderPanel();
    operatorFocus(approveOf(0));
    panelState.history = [resolvedElsewhere(panelState.history[0]), ...panelState.history.slice(1)];
    refreshPanel();
    advance(GRACE);
    expect(announce.mock.calls.filter(([t]) => t === "Answer confirmed, moved to Recent activity")).toHaveLength(1);
    expect(document.activeElement).toBe(container.querySelector('[data-tray-row="n1"]'));
  });

  it("stays silent about a late acknowledgement when focus was elsewhere (D6)", () => {
    const { refreshPanel, announce } = renderPanel();
    panelState.history = [resolvedElsewhere(panelState.history[0]), ...panelState.history.slice(1)];
    refreshPanel();
    advance(GRACE * 2);
    expect(announce.mock.calls.filter(([t]) => String(t).startsWith("Answer confirmed"))).toHaveLength(0);
  });
});
