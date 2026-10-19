import { act, fireEvent, screen, within } from "@testing-library/react";
import { ActionOutcome } from "@/gen/session/v1/notification_action_pb";
import { invokeMock, respondWith } from "./__fixtures__/actionRowHarness";
import { setTimingForTests } from "@/lib/hooks/interactionTiming";
import { pendingApprovals, pendingQuestions, renderPanel, resolvedElsewhere, rowKeys } from "./__fixtures__/panelHarness";
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
const mockConnectivity = { offline: false };
jest.mock("@/lib/hooks/useNotificationConnectivity", () => ({
  useNotificationConnectivity: () => ({ state: "connected", isOffline: mockConnectivity.offline }),
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
  mockConnectivity.offline = false;
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

describe("Story 2.10b: digit and focus guards", () => {
  const rowOf = (container: HTMLElement, key: string) => container.querySelector(`[data-tray-row="${key}"]`) as HTMLElement;
  const press = (key: string, init: KeyboardEventInit = {}, target: Element | null = document.activeElement) =>
    act(() => void fireEvent.keyDown(target as Element, { key, ...init }));
  const optionButtons = (container: HTMLElement, key: string) => Array.from(rowOf(container, key).querySelectorAll<HTMLElement>("[data-action-option]"));

  beforeEach(() => {
    invokeMock.mockReset();
    respondWith(ActionOutcome.APPLIED);
    panelState.history = [...pendingQuestions(2), ...pendingApprovals(1)];
  });

  it("ignores a digit 100 ms after focus advanced to the next card and sends nothing", async () => {
    respondWith(ActionOutcome.APPLIED);
    panelState.history = [...pendingApprovals(1), ...pendingQuestions(1).map((q) => ({ ...q, timestamp: 5_000 }))];
    const { container, refreshPanel } = renderPanel();
    expect(rowKeys(container).filter((k) => k !== null && /^[nq]\d/.test(k))).toEqual(["n0", "q0"]);
    operatorFocus(approveOf(0));
    await act(async () => approveOf(0).click());
    panelState.history = [resolvedElsewhere(panelState.history.find((n) => n.id === "n0")!), ...panelState.history.filter((n) => n.id !== "n0")];
    refreshPanel();
    advance(GRACE);
    const q0 = rowOf(container, "q0");
    expect(document.activeElement).toBe(q0);
    invokeMock.mockClear();
    advance(100);
    press("1");
    expect(document.activeElement).toBe(q0);
    expect(invokeMock).not.toHaveBeenCalled();
  });

  it("lets 1, 2, 3 pressed 50 ms apart visit options in order on an opened card, keeps it engaged and sends nothing until Enter on an option", async () => {
    const { container, refreshPanel } = renderPanel();
    const q1 = rowOf(container, "q1");
    operatorFocus(q1);
    press("Enter");
    advance(GRACE);
    const options = optionButtons(container, "q1");
    expect(options).toHaveLength(3);
    for (const [i, key] of ["1", "2", "3"].entries()) {
      press(key);
      expect(document.activeElement).toBe(options[i]);
      advance(50);
    }
    panelState.history = [...pendingQuestions(1, 9), ...panelState.history];
    refreshPanel();
    expect(screen.getByTestId("tray-new-pill")).toHaveTextContent("1 new");
    expect(invokeMock).not.toHaveBeenCalled();
    press("2");
    await act(async () => options[1].click());
    expect(invokeMock).toHaveBeenCalledTimes(1);
  });

  it("does not arm the pre-expanded first item until the operator tabs onto it", () => {
    const { container } = renderPanel();
    const first = rowOf(container, "q0");
    expect(first).toHaveAttribute("data-expanded", "pre");
    advance(GRACE);
    press("1", {}, first);
    expect(document.activeElement).not.toBe(optionButtons(container, "q0")[0]);
    operatorFocus(first);
    advance(GRACE);
    press("1");
    expect(document.activeElement).toBe(optionButtons(container, "q0")[0]);
  });

  it("collapses the pre-expanded item on Enter and arms it on the next Enter", () => {
    const { container } = renderPanel();
    const first = rowOf(container, "q0");
    operatorFocus(first);
    press("Enter");
    expect(first).not.toHaveAttribute("data-expanded");
    press("Enter");
    expect(first).toHaveAttribute("data-expanded", "operator");
  });

  it("holds an operator-expanded card engaged on touch, 10 s after the tap, until it is collapsed", () => {
    const { container, refreshPanel } = renderPanel();
    const q1 = rowOf(container, "q1");
    fireEvent.touchStart(q1);
    act(() => void fireEvent.click(within(q1).getByText("Something happened")));
    expect(q1).toHaveAttribute("data-expanded", "operator");
    advance(10_000);
    panelState.history = [...pendingQuestions(1, 9), ...panelState.history];
    refreshPanel();
    expect(screen.getByTestId("tray-new-pill")).toHaveTextContent("1 new");
    expect(rowKeys(container).filter((k) => k?.startsWith("q"))).toEqual(["q0", "q1"]);
    press("Escape", {}, q1);
    expect(q1).not.toHaveAttribute("data-expanded");
    advance(GRACE);
    expect(rowKeys(container).filter((k) => k?.startsWith("q"))).toEqual(["q9", "q0", "q1"].sort());
  });

  it.each([
    ["with Ctrl held", { ctrlKey: true }],
    ["during IME composition", { isComposing: true }],
  ])("ignores digits %s", (_name, init) => {
    const { container } = renderPanel();
    const q1 = rowOf(container, "q1");
    operatorFocus(q1);
    press("Enter");
    advance(GRACE);
    press("1", init);
    expect(document.activeElement).toBe(q1);
  });

  it("ignores digits typed in a field inside the card", () => {
    const { container } = renderPanel();
    const q1 = rowOf(container, "q1");
    operatorFocus(q1);
    press("Enter");
    advance(GRACE);
    const field = document.createElement("textarea");
    q1.appendChild(field);
    operatorFocus(field);
    press("1", {}, field);
    expect(document.activeElement).toBe(field);
  });
});

describe("Task 2.10c: closing the panel mid-hold cancels", () => {
  beforeEach(() => {
    invokeMock.mockReset();
    setTimingForTests({ settleMs: 0, commitDelayMs: 3_000 });
    panelState.isOpen = true;
    window.sessionStorage.clear();
  });
  afterEach(() => {
    panelState.isOpen = true;
  });

  it("sends nothing when the panel closes, then shows the cancellation line with the item still counted and unacknowledged", () => {
    const { refreshPanel, announce } = renderPanel();
    act(() => approveOf(0).click());
    expect(screen.getByTestId("action-undo")).toBeInTheDocument();
    panelState.isOpen = false;
    refreshPanel();
    advance(10_000);
    panelState.isOpen = true;
    refreshPanel();

    expect(invokeMock).not.toHaveBeenCalled();
    expect(panelState.acknowledge).not.toHaveBeenCalled();
    expect(screen.getByTestId("hold-cancelled")).toHaveTextContent("Approval cancelled when you left. Tap Approve to try again.");
    expect(screen.getByTestId("tray-needs-attention")).toHaveTextContent("3 need you");
    expect(announce.mock.calls.filter(([t]) => t === "Approval undone")).toHaveLength(1);
  });

  it("holds the Needs-you list still while the hold runs, with focus on Undo", () => {
    const { container, refreshPanel } = renderPanel();
    act(() => approveOf(0).click());
    panelState.history = [...pendingApprovals(1, 9), ...panelState.history];
    refreshPanel();
    expect(screen.getByTestId("tray-new-pill")).toHaveTextContent("1 new");
    expect(needsYou(container)).toEqual(["n0", "n1", "n2"]);
  });
});

describe("Task 2.10d: reserved status slot and fling guard", () => {
  const rowOffsets = (container: HTMLElement) =>
    Array.from(container.querySelectorAll<HTMLElement>("[data-tray-row]")).map((el) => `${el.dataset.trayRow}@${el.parentElement?.style.transform}`);

  beforeEach(() => {
    invokeMock.mockReset();
    setTimingForTests({ settleMs: 0, commitDelayMs: 0 });
  });

  it("renders the status slot from first paint, outside the scroll region, and moves no row when the banner and the N new pill appear", () => {
    const { container, refreshPanel } = renderPanel();
    const slot = screen.getByTestId("tray-status-slot");
    expect(screen.getByTestId("tray-scroll").contains(slot)).toBe(false);
    expect(slot).toContainElement(screen.getByTestId("tray-as-of"));
    const before = rowOffsets(container);

    operatorFocus(approveOf(0));
    panelState.history = [...pendingApprovals(1, 9), ...panelState.history];
    refreshPanel();
    expect(slot).toContainElement(screen.getByTestId("tray-new-pill"));
    expect(rowOffsets(container)).toEqual(before);

    mockConnectivity.offline = true;
    refreshPanel();
    expect(slot).toContainElement(screen.getByTestId("tray-banner-offline"));
    expect(rowOffsets(container)).toEqual(before);
  });

  it("swallows a touch that begins 100 ms after a scroll event and activates one that begins at 200 ms", () => {
    renderPanel();
    const scroll = screen.getByTestId("tray-scroll");
    const deny = screen.getAllByTestId("action-deny")[0];
    act(() => void fireEvent.scroll(scroll));
    advance(100);
    act(() => {
      fireEvent.pointerDown(deny);
      fireEvent.click(deny);
    });
    expect(screen.queryByTestId("deny-step")).toBeNull();

    act(() => void fireEvent.scroll(scroll));
    advance(200);
    act(() => {
      fireEvent.pointerDown(deny);
      fireEvent.click(deny);
    });
    expect(screen.getByTestId("deny-step")).toBeInTheDocument();
    expect(invokeMock).not.toHaveBeenCalled();
  });
});
