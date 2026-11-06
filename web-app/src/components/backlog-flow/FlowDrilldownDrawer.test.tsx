import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { GetFlowSummaryResponseSchema, ListFlowDrilldownResponseSchema } from "@/gen/session/v1/backlog_flow_pb";
import { FlowPage } from "@/app/backlog/flow/FlowPage";
import { AS_OF, location, makeSummary, renderAt } from "@/app/backlog/flow/__tests__/flowFixtures";
import { FlowDrilldownDrawer } from "./FlowDrilldownDrawer";

const mockClient = { getFlowSummary: jest.fn(), compareFlowWindows: jest.fn(), getItemFlowTimeline: jest.fn(), listFlowDrilldown: jest.fn() };

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({ ...jest.requireActual("@connectrpc/connect"), createClient: () => mockClient }));

const HOUR = 3_600_000;
const drillItem = (i: number) => ({ itemId: `item-${i}`, title: `Fix login test ${i}`, status: "review", reason: "FAIL x3", durationMs: BigInt(Math.round(2.1 * 24 * HOUR)) });

function drilldown(count: number, page: object = {}) {
  return create(ListFlowDrilldownResponseSchema, {
    key: "rework.review_fail",
    meta: { asOf: timestampFromDate(AS_OF) },
    items: { items: Array.from({ length: count }, (_, i) => drillItem(i)), totalCount: count, returnedCount: count, ...page },
  } as never);
}

const openDrawer = (listHrefFor?: (key: string) => string | null, onClose = jest.fn()) =>
  renderAt(
    "/backlog/flow?window=30",
    <FlowDrilldownDrawer drillKey="rework.review_fail" window={30} returnFocusTo={null} onClose={onClose} listHrefFor={listHrefFor} />,
  );
const drawer = () => screen.findByTestId("flow-drilldown-drawer");

beforeEach(() => {
  mockClient.listFlowDrilldown.mockReset().mockResolvedValue(drilldown(12));
  mockClient.getFlowSummary.mockReset().mockResolvedValue(create(GetFlowSummaryResponseSchema, { summary: makeSummary({ tiles: { meta: { status: "ok" }, merged: { value: 9, drillKey: "tiles.merged" } } }) }));
  mockClient.compareFlowWindows.mockReset();
});

describe("FlowDrilldownDrawer", () => {
  it("FlowDrilldownDrawer_should_ListItemsUnderHeadingWithCount_When_DrillOpened", async () => {
    await openDrawer();

    const dialog = await drawer();
    expect(await within(dialog).findByRole("heading", { name: "Review-FAIL rework, last 30d, 12 items" })).toBeInTheDocument();
    expect(mockClient.listFlowDrilldown).toHaveBeenCalledWith(expect.objectContaining({ key: "rework.review_fail", windowDays: 30, allTime: false, offset: 0 }));
    const row = within(dialog).getByRole("row", { name: /Fix login test 3/ });
    expect(row).toHaveTextContent("review");
    expect(row).toHaveTextContent("FAIL x3");
    expect(row).toHaveTextContent("2.1d");
    expect(within(row).getByRole("link", { name: "Fix login test 3" })).toHaveAttribute("href", "?window=30&item=item-3");
    expect(dialog).toHaveAttribute("role", "dialog");
  });

  it("FlowDrilldownDrawer_should_ShowErrorWithRetryAndClose_When_DrilldownRpcFails", async () => {
    const user = userEvent.setup();
    const onClose = jest.fn();
    mockClient.listFlowDrilldown.mockRejectedValueOnce(new Error("boom"));
    await openDrawer(undefined, onClose);

    const dialog = await drawer();
    expect(await within(dialog).findByText(/Could not load items/)).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Retry" }));
    expect(await within(dialog).findByText("Fix login test 0")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("FlowDrilldownDrawer_should_ShowEveryStateWithAction_When_LoadingErrorEmptyMismatchTruncatedOrBadKey", async () => {
    const user = userEvent.setup();
    mockClient.listFlowDrilldown.mockReturnValueOnce(new Promise(() => {}));
    const { unmount } = await openDrawer();
    expect(await drawer()).toHaveAttribute("aria-busy", "true");
    expect(screen.getByTestId("flow-dialog-skeleton")).toHaveAttribute("aria-busy", "true");
    unmount();

    mockClient.listFlowDrilldown.mockResolvedValueOnce(drilldown(0, { totalCount: 12 })).mockResolvedValueOnce(drilldown(12));
    const empty = await openDrawer();
    const dialog = await drawer();
    expect(await within(dialog).findByText(/No items match \(the count may have changed since as-of 14:02\)/)).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Refresh" }));
    expect(await within(dialog).findByText("Fix login test 0")).toBeInTheDocument();
    empty.unmount();

    mockClient.listFlowDrilldown.mockResolvedValueOnce(drilldown(10, { totalCount: 12 }));
    const mismatch = await openDrawer();
    expect(await within(await drawer()).findByText(/Counts changed since 14:02; refresh/)).toBeInTheDocument();
    mismatch.unmount();

    mockClient.listFlowDrilldown.mockResolvedValueOnce(drilldown(100, { totalCount: 240, truncated: true }));
    await openDrawer();
    expect(await within(await drawer()).findByText(/Showing 100 of 240 items/)).toBeInTheDocument();
  });

  it("FlowDrilldownDrawer_should_ShowTruncationAndMismatchNotes_When_Over100Rows", async () => {
    mockClient.listFlowDrilldown.mockResolvedValue(drilldown(100, { totalCount: 240, truncated: true }));
    const view = await openDrawer();
    const dialog = await drawer();
    expect(await within(dialog).findByText(/Showing 100 of 240 items/)).toBeInTheDocument();
    expect(within(dialog).queryByRole("link", { name: "Open full list in List view" })).toBeNull();
    expect(within(dialog).queryByRole("button", { name: "Load more" })).toBeNull();
    view.unmount();

    await openDrawer((key) => (key === "rework.review_fail" ? "/backlog?status=review" : null));
    expect(await screen.findByRole("link", { name: "Open full list in List view" })).toHaveAttribute("href", "/backlog?status=review");
  });

  it("FlowDrilldownDrawer_should_AppendNextPage_When_LoadMoreActivated", async () => {
    const user = userEvent.setup();
    mockClient.listFlowDrilldown
      .mockResolvedValueOnce(drilldown(50, { totalCount: 80, hasMore: true }))
      .mockResolvedValueOnce(create(ListFlowDrilldownResponseSchema, { items: { items: Array.from({ length: 30 }, (_, i) => drillItem(50 + i)), totalCount: 80, hasMore: false } } as never));
    await openDrawer();
    const dialog = await drawer();
    expect(await within(dialog).findByText(/Showing 50 of 80 items/)).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Load more" }));

    await waitFor(() => expect(within(dialog).getAllByRole("row")).toHaveLength(81));
    expect(mockClient.listFlowDrilldown).toHaveBeenLastCalledWith(expect.objectContaining({ offset: 50 }));
    expect(within(dialog).queryByText(/Showing/)).toBeNull();
  });
});

describe("FlowPage drill URL", () => {
  it("FlowDrilldownDrawer_should_RemoveDrillParam_When_Closed", async () => {
    const user = userEvent.setup();
    await renderAt("/backlog/flow?drill=rework.review_fail", <FlowPage />);
    const dialog = await drawer();
    await within(dialog).findByText("Fix login test 0");

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByTestId("flow-drilldown-drawer")).toBeNull());
    expect(location()).toBe("/backlog/flow");
  });

  it("FlowDrilldownDrawer_should_RemoveDrillParam_When_ClosedByXOrCloseButton", async () => {
    const user = userEvent.setup();
    await renderAt("/backlog/flow?drill=rework.review_fail&window=7", <FlowPage />);
    await within(await drawer()).findByText("Fix login test 0");
    await user.click(screen.getByRole("button", { name: "Close dialog" }));
    await waitFor(() => expect(screen.queryByTestId("flow-drilldown-drawer")).toBeNull());
    expect(location()).toBe("/backlog/flow?window=7");

    await user.click(screen.getByRole("button", { name: /9 merged/ }));
    await within(await drawer()).findByText("Fix login test 0");
    await user.click(within(screen.getByTestId("flow-drilldown-drawer")).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByTestId("flow-drilldown-drawer")).toBeNull());
    expect(location()).toBe("/backlog/flow?window=7");
  });

  it("FlowDrilldownDrawer_should_IgnoreUnknownKeyAndShowNotice_When_DrillParamBogus", async () => {
    await renderAt("/backlog/flow?drill=bogus", <FlowPage />);

    expect(await screen.findByText("Unknown drill target ignored")).toBeInTheDocument();
    expect(screen.queryByTestId("flow-drilldown-drawer")).toBeNull();
    expect(mockClient.listFlowDrilldown).not.toHaveBeenCalled();
    expect(location()).toBe("/backlog/flow");
  });
});

const HOUR_MS = BigInt(HOUR);
const dwell = { n: 10, p50Ms: 2n * HOUR_MS, p90Ms: 24n * HOUR_MS };

function richSummary() {
  return makeSummary({
    tiles: {
      meta: { status: "ok" },
      merged: { value: 9, drillKey: "tiles.merged" },
      firstPassYield: { num: 32, den: 105, drillKey: "fpy.first_pass" },
      leadTime: { n: 30, p50Ms: 70n * HOUR_MS, p90Ms: 190n * HOUR_MS, drillKey: "tiles.lead_time" },
    },
    funnel: {
      meta: { status: "ok" },
      stages: [
        { status: "in_progress", entered: { value: 141, drillKey: "funnel.stage.in_progress" }, advanced: 105, backEdges: 4, dwell },
        { status: "review", entered: { value: 105, drillKey: "funnel.stage.review" }, advanced: 90, backEdges: 0, dwell },
      ],
      other: 3,
      thrash: { n: 7 },
    },
    causes: { meta: { status: "ok" }, mergedCount: 105, rework: [{ cause: "review_fail", count: { value: 398, drillKey: "rework.review_fail" } }] },
    review: {
      meta: { status: "ok" },
      total: { value: 528, drillKey: "review.class.judgment" },
      rawFail: { num: 186, den: 528, drillKey: "review.class.judgment" },
      adjustedFail: { num: 88, den: 355, drillKey: "review.class.judgment" },
      classes: [{ class: "no_commits", total: { value: 98, drillKey: "review.class.no_commits" } }],
    },
    pr: { meta: { status: "ok" }, fixDwell: { ...dwell, drillKey: "pr.fix_dwell" }, cycles: { value: 103, drillKey: "pr.fix_dwell" } },
    stuck: {
      meta: { status: "ok" },
      open: 8,
      reasons: [{ reason: "needs_input", open: 3, distinctItems: { value: 5, drillKey: "stuck.needs_input" }, duration: { ...dwell, drillKey: "stuck.needs_input" } }],
    },
    anomalies: {
      meta: { status: "ok" },
      thrashItems: { value: 1, drillKey: "thrash.episode" },
      items: { items: [{ itemId: "a90b99cb", title: "Flaky review", episodes: 5, events: 8700, pair: "a <-> b" }], totalCount: 1, returnedCount: 1 },
    },
  });
}

// Not item sets: charts and the funnel's visually hidden table (mirrors the drillable bars), Data notes, the as-of clock and the /unfinished link.
const NOT_DRILLABLE = "figure, [data-testid='flow-funnel-table'], [data-testid='flow-section-notes'], [data-testid='flow-tile-open-stuck'], [data-testid='flow-as-of']";
const NUMERIC_TEXT = /^\(?[\d,.]+%?\)?$/;

function orphanNumbers(root: HTMLElement): string[] {
  const orphans: string[] = [];
  for (const element of root.querySelectorAll("*")) {
    const own = [...element.childNodes]
      .filter((node) => node.nodeType === Node.TEXT_NODE)
      .map((node) => node.textContent ?? "")
      .join("")
      .trim();
    if (NUMERIC_TEXT.test(own) && !element.closest(`button, a, ${NOT_DRILLABLE}`)) orphans.push(`${element.tagName.toLowerCase()}: ${own}`);
  }
  return orphans;
}

describe("FlowPage drill coverage", () => {
  it("FlowPage_should_HaveDrillButtonForEveryNumber_When_AllSectionsRendered", async () => {
    const user = userEvent.setup();
    mockClient.getFlowSummary.mockResolvedValue(create(GetFlowSummaryResponseSchema, { summary: richSummary() }));
    await renderAt("/backlog/flow", <FlowPage />);
    await user.click(await screen.findByTestId("flow-toggle-stuck"));
    await user.click(screen.getByTestId("flow-toggle-anomalies"));

    const sections = screen.getAllByTestId(/^flow-section-/).filter((section) => section.dataset.testid !== "flow-section-notes");
    expect(sections.length).toBeGreaterThanOrEqual(7);
    expect(sections.flatMap(orphanNumbers)).toEqual([]);
  });
});
