import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { CompareFlowWindowsResponseSchema, GetFlowSummaryResponseSchema, GetItemFlowTimelineResponseSchema } from "@/gen/session/v1/backlog_flow_pb";
import { FLOW_POLL_INTERVAL_MS } from "@/lib/hooks/useBacklogFlow";
import { FlowPage } from "../FlowPage";
import { AS_OF, location, makeSummary, renderAt, resourceExhausted } from "./flowFixtures";

const mockClient = { getFlowSummary: jest.fn(), compareFlowWindows: jest.fn(), getItemFlowTimeline: jest.fn(), listFlowDrilldown: jest.fn() };

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => mockClient,
}));

const respond = (overrides = {}) => create(GetFlowSummaryResponseSchema, { summary: makeSummary(overrides) });
const heading = (name: string) => screen.getByRole("heading", { level: 2, name });

function setHidden(hidden: boolean) {
  Object.defineProperty(document, "hidden", { configurable: true, get: () => hidden });
}

beforeEach(() => {
  jest.useFakeTimers({ now: AS_OF.getTime() + 12_000 });
  mockClient.getFlowSummary.mockReset().mockResolvedValue(respond());
  mockClient.compareFlowWindows.mockReset().mockResolvedValue(create(CompareFlowWindowsResponseSchema, {}));
  setHidden(false);
});

afterEach(() => {
  jest.useRealTimers();
});

describe("FlowPage", () => {
  it("FlowPage_should_ShowSkeletonsAndNoZero_When_FirstLoadPending", async () => {
    mockClient.getFlowSummary.mockReturnValue(new Promise(() => {}));
    await renderAt("/backlog/flow", <FlowPage />);

    expect(screen.getByTestId("flow-skeleton-tiles")).toBeInTheDocument();
    expect(screen.getByTestId("flow-as-of")).toHaveTextContent("Loading...");
    expect(screen.getByTestId("flow-section-tiles")).not.toHaveTextContent(/\b0\b/);
    expect(screen.getByRole("link", { name: "Flow" })).toHaveAttribute("aria-current", "page");
  });

  it("FlowPage_should_ShowServerAsOfAndRelativeAge_When_ClientClockDiffers", async () => {
    await renderAt("/backlog/flow", <FlowPage />);

    expect(screen.getByTestId("flow-as-of")).toHaveTextContent("As of 14:02:11 (12s ago)");
    expect(screen.getByTestId("flow-as-of")).toHaveAttribute("title", expect.stringContaining("14:02:11"));
  });

  it("FlowPage_should_DimDataAndShowRetryBanner_When_RefreshFailsAfterSuccess", async () => {
    await renderAt("/backlog/flow", <FlowPage />);
    mockClient.getFlowSummary.mockRejectedValueOnce(new Error("boom"));

    fireEvent.click(screen.getByTestId("flow-refresh"));
    const banner = await screen.findByTestId("flow-stale-banner");

    expect(banner).toHaveTextContent("Stale");
    expect(banner).toHaveTextContent("Showing data from 14:02 - refresh failed");
    expect(screen.getByTestId("flow-section-tiles").parentElement).toHaveAttribute("data-stale", "true");

    fireEvent.click(within(banner).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(screen.queryByTestId("flow-stale-banner")).toBeNull());
  });

  it("FlowPage_should_BeStaleAt121sNotAt119s_When_PollsFailWithFakeClock", async () => {
    expect(FLOW_POLL_INTERVAL_MS).toBe(60_000);
    await renderAt("/backlog/flow", <FlowPage />);
    mockClient.getFlowSummary.mockRejectedValue(new Error("down"));

    await act(async () => void (await jest.advanceTimersByTimeAsync(119_000)));
    expect(mockClient.getFlowSummary).toHaveBeenCalledTimes(2);
    expect(screen.queryByTestId("flow-stale-banner")).toBeNull();

    await act(async () => void (await jest.advanceTimersByTimeAsync(2_000)));
    expect(screen.getByTestId("flow-stale-banner")).toBeInTheDocument();

    mockClient.getFlowSummary.mockResolvedValue(respond());
    await act(async () => void (await jest.advanceTimersByTimeAsync(FLOW_POLL_INTERVAL_MS)));
    expect(screen.queryByTestId("flow-stale-banner")).toBeNull();
  });

  it("FlowPage_should_PausePolling_When_TabHidden", async () => {
    await renderAt("/backlog/flow", <FlowPage />);
    setHidden(true);

    await act(async () => void (await jest.advanceTimersByTimeAsync(FLOW_POLL_INTERVAL_MS * 3)));

    expect(mockClient.getFlowSummary).toHaveBeenCalledTimes(1);
  });

  it("FlowPage_should_ShowErrorStateWithRetryAndBoardLink_When_NoPriorData", async () => {
    mockClient.getFlowSummary.mockRejectedValueOnce(new Error("flow aggregator is not available"));
    await renderAt("/backlog/flow", <FlowPage />);

    expect(await screen.findByText("flow aggregator is not available")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back to Board" })).toHaveAttribute("href", "/backlog/board");
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(screen.queryByTestId("flow-hard-error")).toBeNull());
    expect(heading("Review gate")).toBeInTheDocument();
  });

  it("FlowPage_should_RenderOtherSections_When_ReviewSectionFailed", async () => {
    mockClient.getFlowSummary.mockResolvedValue(respond({ review: { meta: { status: "failed", error: "query timed out" } } }));
    await renderAt("/backlog/flow", <FlowPage />);

    const block = await screen.findByTestId("flow-section-error-review");
    expect(block).toHaveTextContent("Review section failed to load: query timed out");
    expect(screen.queryByTestId("flow-section-error-causes")).toBeNull();
    expect(screen.getByTestId("flow-unavailable-count")).toHaveTextContent("1 section unavailable");
    expect(screen.queryByRole("button", { name: "Retry section" })).toBeNull();

    mockClient.getFlowSummary.mockResolvedValue(respond());
    fireEvent.click(within(block).getByRole("button", { name: "Reload" }));
    await waitFor(() => expect(screen.queryByTestId("flow-section-error-review")).toBeNull());
    expect(mockClient.getFlowSummary).toHaveBeenCalledTimes(2);
  });

  it("FlowPage_should_RenderAllSections_When_SuccessResponse", async () => {
    await renderAt("/backlog/flow?window=30", <FlowPage />);

    await screen.findByRole("group", { name: "Health summary" });
    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    const sections = screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent);
    expect(sections).toEqual(expect.arrayContaining(["Review gate", "Anomalies & thrash"]));
    expect(sections.length).toBeGreaterThanOrEqual(6);
  });

  it("FlowPage_should_ShowWidenButtonAndDenominator_When_EmptyWindowAndZeroFails", async () => {
    mockClient.getFlowSummary.mockResolvedValue(
      respond({
        tiles: { meta: { status: "empty" } },
        review: { meta: { status: "ok" }, total: { value: 14 }, rawFail: { num: 0, den: 14, ratio: 0 }, adjustedFail: { num: 0, den: 14, ratio: 0 } },
      }),
    );
    await renderAt("/backlog/flow?window=7", <FlowPage />);

    expect(await screen.findByText("No items finished in the last 7 days")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Widen to 30d" })).toBeInTheDocument();
    expect(screen.getByTestId("flow-review-rates")).toHaveTextContent("0/14 (0.0%)");
  });

  it("FlowPage_should_ShowWidenButton_When_EmptyWindow", async () => {
    mockClient.getFlowSummary.mockResolvedValue(respond({ tiles: { meta: { status: "empty" } } }));
    await renderAt("/backlog/flow?window=7", <FlowPage />);

    expect(await screen.findByText("No items finished in the last 7 days")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Widen to 30d" }));
    expect(location()).toBe("/backlog/flow?window=30");
  });

  it("FlowPage_should_IgnoreUnknownWindowAndDrillWithNote_When_BadUrlParams", async () => {
    await renderAt("/backlog/flow?window=13&drill=bogus&item=abc", <FlowPage />);

    expect(await screen.findByText("Unsupported window, showing 30d")).toBeInTheDocument();
    expect(screen.getByText("Unknown drill target ignored")).toBeInTheDocument();
    expect(location()).toBe("/backlog/flow?item=abc");
    expect(mockClient.getFlowSummary.mock.calls[0][0]).toMatchObject({ windowDays: 30, allTime: false });
  });

  it("FlowPage_should_ReflectUrlSelection_When_LoadedWithWindowAndCompare", async () => {
    await renderAt("/backlog/flow?window=90&compare=prev&drill=rework.review_fail", <FlowPage />);

    expect(screen.getByTestId("flow-window-90")).toHaveAttribute("aria-checked", "true");
    expect(screen.getByTestId("flow-compare-prev")).toHaveAttribute("aria-checked", "true");
    expect(location()).toBe("/backlog/flow?window=90&compare=prev&drill=rework.review_fail");
    await waitFor(() => expect(mockClient.compareFlowWindows).toHaveBeenCalledWith(expect.objectContaining({ aDays: 90, bDays: 90, bOffsetDays: 0 })));
  });

  it("FlowPage_should_MoveSelectionWithArrowKeys_When_RadioGroupFocused", async () => {
    await renderAt("/backlog/flow?window=30", <FlowPage />);

    expect(screen.getByRole("radiogroup", { name: "Window" })).toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("radiogroup", { name: "Window" }), { key: "ArrowRight" });
    expect(location()).toBe("/backlog/flow?window=90");
    expect(screen.getByTestId("flow-window-90")).toHaveAttribute("aria-checked", "true");

    fireEvent.keyDown(screen.getByRole("radiogroup", { name: "Window" }), { key: "ArrowRight" });
    expect(location()).toBe("/backlog/flow?window=all");
    fireEvent.keyDown(screen.getByRole("radiogroup", { name: "Window" }), { key: "ArrowLeft" });
    expect(location()).toBe("/backlog/flow?window=90");
  });

  it("FlowPage_should_RestorePriorSelection_When_BrowserBack", async () => {
    await renderAt("/backlog/flow?window=30&compare=prev", <FlowPage />);

    fireEvent.click(screen.getByRole("radio", { name: "90d" }));
    expect(location()).toBe("/backlog/flow?window=90&compare=prev");
    fireEvent.click(screen.getByTestId("history-back"));

    expect(location()).toBe("/backlog/flow?window=30&compare=prev");
    expect(screen.getByTestId("flow-window-30")).toHaveAttribute("aria-checked", "true");
  });

  it("FlowPage_should_AnnounceOnceOnRefreshAndStayQuiet_When_BackgroundPoll", async () => {
    const { announce } = await renderAt("/backlog/flow", <FlowPage />);
    expect(announce).toHaveBeenCalledTimes(1);
    expect(announce).toHaveBeenLastCalledWith("Flow updated, as of 14:02", "polite", "flow-updated");

    await act(async () => void (await jest.advanceTimersByTimeAsync(FLOW_POLL_INTERVAL_MS * 2)));
    expect(mockClient.getFlowSummary).toHaveBeenCalledTimes(3);
    expect(announce).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByTestId("flow-refresh"));
    expect(screen.getByTestId("flow-refresh")).toHaveTextContent("Refreshing...");
    expect(screen.getByTestId("flow-refresh")).toHaveAttribute("aria-disabled", "true");
    await waitFor(() => expect(announce).toHaveBeenCalledTimes(2));
    expect(screen.getByTestId("flow-refresh")).toHaveTextContent("Refresh");
  });

  it("FlowPage_should_HaveOneH1AndFocusHeading_When_JumpLinkActivated", async () => {
    await renderAt("/backlog/flow", <FlowPage />);

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    fireEvent.click(within(screen.getByRole("navigation", { name: "Jump to section" })).getByRole("link", { name: "Review" }));

    await waitFor(() => expect(heading("Review gate")).toHaveFocus());
    expect(heading("Review gate")).toHaveAttribute("tabindex", "-1");
    expect(location()).toBe("/backlog/flow");
  });

  it("FlowPage_should_CollapseStuckAndAnomaliesAndExpandOnJumpOrFailure_When_Rendered", async () => {
    await renderAt("/backlog/flow", <FlowPage />);

    expect(screen.getByTestId("flow-toggle-stuck")).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByTestId("flow-toggle-anomalies")).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByTestId("flow-section-stuck")).toHaveTextContent("8 open, 46 historical episodes");
    expect(screen.getByTestId("flow-section-anomalies")).toHaveTextContent("35 items thrashing");
    expect(screen.queryByTestId("flow-toggle-review")).toBeNull();

    fireEvent.click(within(screen.getByRole("navigation", { name: "Jump to section" })).getByRole("link", { name: "Anomalies" }));
    await waitFor(() => expect(heading("Anomalies & thrash")).toHaveFocus());
    expect(screen.getByTestId("flow-toggle-anomalies")).toHaveAttribute("aria-expanded", "true");
  });

  it("FlowPage_should_RenderFailedCollapsedSectionExpanded_When_StuckFailed", async () => {
    mockClient.getFlowSummary.mockResolvedValue(respond({ stuck: { meta: { status: "failed", error: "db locked" } } }));
    await renderAt("/backlog/flow", <FlowPage />);

    expect(await screen.findByTestId("flow-section-error-stuck")).toHaveTextContent("db locked");
    expect(screen.getByTestId("flow-toggle-stuck")).toHaveAttribute("aria-expanded", "true");
  });

  it("FlowPage_should_OpenDataNotesAndFocusIt_When_EstimatedLinkActivated", async () => {
    mockClient.getFlowSummary.mockResolvedValue(respond({ meta: { asOf: timestampFromDate(AS_OF), estimated: ["a", "b"] } }));
    await renderAt("/backlog/flow", <FlowPage />);

    fireEvent.click(await screen.findByRole("button", { name: "2 metrics estimated" }));

    await waitFor(() => expect(heading("Data notes")).toHaveFocus());
    expect(screen.getByTestId("flow-toggle-notes")).toHaveAttribute("aria-expanded", "true");
  });

  it("FlowPage_should_ShowWindowTooLargeStateWithShorterWindowButtons_When_AllTimeResourceExhausted", async () => {
    mockClient.getFlowSummary.mockRejectedValue(resourceExhausted("window too large (300000 rows, limit 250000); use a shorter window"));
    await renderAt("/backlog/flow?window=all", <FlowPage />);

    const state = await screen.findByTestId("flow-window-too-large");
    expect(state).toHaveTextContent("All time has too many events to load (300000 rows, limit 250000). Pick a shorter window.");
    expect(within(state).getAllByRole("button").map((b) => b.textContent)).toEqual(["7d", "30d", "90d"]);
    expect(within(state).getByRole("button", { name: "7d" })).toHaveFocus();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.getByTestId("flow-window-all")).toBeInTheDocument();

    mockClient.getFlowSummary.mockResolvedValue(respond());
    fireEvent.click(within(state).getByRole("button", { name: "30d" }));
    expect(location()).toBe("/backlog/flow?window=30");
    await waitFor(() => expect(screen.queryByTestId("flow-window-too-large")).toBeNull());
  });

  it("FlowPage_should_TreatBusyResourceExhaustedAsRetryableError_When_NotWindowTooLarge", async () => {
    mockClient.getFlowSummary.mockRejectedValueOnce(resourceExhausted("flow aggregator is busy"));
    await renderAt("/backlog/flow?window=all", <FlowPage />);

    expect(await screen.findByText("flow aggregator is busy")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
    expect(screen.queryByTestId("flow-window-too-large")).toBeNull();
  });

  it.each([
    ["error", () => mockClient.getFlowSummary.mockRejectedValue(new Error("x")), "Retry"],
    ["empty", () => mockClient.getFlowSummary.mockResolvedValue(respond({ tiles: { meta: { status: "empty" } } })), "Widen to 30d"],
    ["partial failure", () => mockClient.getFlowSummary.mockResolvedValue(respond({ pr: { meta: { status: "failed", error: "x" } } })), "Reload"],
  ])("FlowPage_should_OfferAnActionInEveryState_When_%s", async (_name, arrange, action) => {
    arrange();
    await renderAt("/backlog/flow?window=7", <FlowPage />);

    expect(await screen.findByRole("button", { name: action })).toBeInTheDocument();
  });

  it("FlowPage_should_RenderSectionBodiesAndOpenAnomaliesFromBanner_When_ThrashPresent", async () => {
    await renderAt("/backlog/flow", <FlowPage />);

    expect(screen.getByRole("group", { name: "Health summary" })).toBeInTheDocument();
    expect(screen.getByTestId("flow-thrash-banner")).toHaveTextContent("35 items thrashing; excluded from averages");
    expect(screen.queryByText("No thrash episodes in this window")).toBeNull();

    fireEvent.click(within(screen.getByTestId("flow-thrash-banner")).getByRole("button", { name: "View" }));
    expect(await screen.findByText("No thrash episodes in this window")).toBeInTheDocument();
    await waitFor(() => expect(document.activeElement).toBe(heading("Anomalies & thrash")));
  });

  it("FlowPage_should_ReplaceBannerWithUnavailableReload_When_AnomalyDataFailed", async () => {
    mockClient.getFlowSummary.mockResolvedValue(respond({ anomalies: { meta: { status: "failed", error: "boom" } } }));
    await renderAt("/backlog/flow", <FlowPage />);

    const banner = await screen.findByTestId("flow-thrash-banner");
    expect(banner).toHaveTextContent("Anomaly data unavailable");
    expect(screen.getByTestId("flow-section-error-anomalies")).toBeInTheDocument();

    mockClient.getFlowSummary.mockResolvedValue(respond());
    fireEvent.click(within(banner).getByRole("button", { name: "Reload" }));
    await waitFor(() => expect(screen.getByTestId("flow-thrash-banner")).toHaveTextContent("35 items thrashing"));
  });

  it("FlowPage_should_SetDrillParam_When_NumberClicked", async () => {
    mockClient.getFlowSummary.mockResolvedValue(respond({ tiles: { meta: { status: "ok" }, merged: { value: 9, drillKey: "tiles.merged" } } }));
    await renderAt("/backlog/flow", <FlowPage />);

    fireEvent.click(within(await screen.findByTestId("flow-tile-throughput")).getByRole("button", { name: /9 merged/ }));
    expect(location()).toBe("/backlog/flow?drill=tiles.merged");
  });

  it("FlowPage_should_OpenItemTimelineAndReturnFocus_When_ViewEventsActivated", async () => {
    const anomalies = {
      meta: { status: "ok" },
      thrashItems: { value: 1, drillKey: "thrash.episode" },
      items: { items: [{ itemId: "a90b99cb", title: "Flaky review", episodes: 2, events: 40, pair: "a <-> b" }], totalCount: 1, returnedCount: 1 },
    };
    mockClient.getFlowSummary.mockResolvedValue(respond({ anomalies }));
    mockClient.getItemFlowTimeline.mockResolvedValue(create(GetItemFlowTimelineResponseSchema, { itemId: "a90b99cb", title: "Flaky review", entries: [], totalEntries: 0 }));
    await renderAt("/backlog/flow", <FlowPage />);
    fireEvent.click(await screen.findByTestId("flow-toggle-anomalies"));

    const invoker = screen.getByRole("button", { name: /View events/ });
    fireEvent.click(invoker);
    const dialog = await screen.findByTestId("flow-item-timeline");
    expect(mockClient.getItemFlowTimeline).toHaveBeenCalledWith(expect.objectContaining({ itemId: "a90b99cb", windowDays: 30 }));

    fireEvent.keyDown(dialog, { key: "Escape" });
    await waitFor(() => expect(screen.queryByTestId("flow-item-timeline")).toBeNull());
    expect(invoker).toHaveFocus();
  });
});
