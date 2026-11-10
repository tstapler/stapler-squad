import { useState } from "react";
import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FlowAnomaliesSectionSchema, GetItemFlowTimelineResponseSchema } from "@/gen/session/v1/backlog_flow_pb";
import { FlowAnomalies } from "./FlowAnomalies";
import { FlowItemTimeline } from "./FlowItemTimeline";

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
const mockClient = { getItemFlowTimeline: jest.fn() };
jest.mock("@connectrpc/connect", () => ({ ...jest.requireActual("@connectrpc/connect"), createClient: () => mockClient }));

const NOW = new Date(2026, 10, 6, 14, 2, 11).getTime();
const anomalies = create(FlowAnomaliesSectionSchema, {
  meta: { status: "ok" },
  thrashItems: { value: 1, drillKey: "thrash.episode" },
  items: {
    items: [{ itemId: "a90b99cb1234", title: "Flaky review", episodes: 5, events: 8700, pair: "pr_pending <-> in_progress", firstAt: timestampFromDate(new Date(NOW - 86_400_000)), lastAt: timestampFromDate(new Date(NOW - 60_000)) }],
    totalCount: 1,
    returnedCount: 1,
  },
} as never);

const entry = (i: number) => ({ kind: "thrash_episode", from: "pr_pending", to: "in_progress", at: timestampFromDate(new Date(NOW - i * 3_600_000)), last: timestampFromDate(new Date(NOW - i * 3_600_000 + 60_000)), events: 40 + i, cause: "pr_fix" });
const timeline = (count: number, total = count) =>
  create(GetItemFlowTimelineResponseSchema, { itemId: "a90b99cb1234", title: "Flaky review", entries: Array.from({ length: count }, (_, i) => entry(i)), totalEntries: total } as never);

function Harness() {
  const [open, setOpen] = useState<{ itemId: string; invoker: HTMLElement } | null>(null);
  return (
    <>
      <FlowAnomalies anomalies={anomalies} archivedIds={new Set()} now={NOW} onDrill={jest.fn()} onViewEvents={(itemId, invoker) => setOpen({ itemId, invoker })} />
      {open && <FlowItemTimeline itemId={open.itemId} window={30} returnFocusTo={open.invoker} onClose={() => setOpen(null)} />}
    </>
  );
}

beforeEach(() => mockClient.getItemFlowTimeline.mockReset());

describe("FlowItemTimeline", () => {
  it("FlowItemTimeline_should_ShowCollapsedSpansAndRetry_When_ViewEventsActivated", async () => {
    const user = userEvent.setup();
    mockClient.getItemFlowTimeline.mockRejectedValueOnce(new Error("boom")).mockResolvedValueOnce(timeline(3));
    render(<Harness />);
    const invoker = screen.getByRole("button", { name: /View events/ });

    await user.click(invoker);
    const dialog = await screen.findByTestId("flow-item-timeline");
    expect(mockClient.getItemFlowTimeline).toHaveBeenCalledWith(expect.objectContaining({ itemId: "a90b99cb1234", windowDays: 30, allTime: false }));
    expect(await within(dialog).findByText(/Could not load events/)).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Retry" }));
    const rows = await within(dialog).findAllByRole("listitem");
    expect(rows).toHaveLength(3);
    expect(rows[0]).toHaveTextContent("Thrash episode: pr_pending <-> in_progress, 40 events");
    expect(dialog).toHaveTextContent("Events for Flaky review");

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByTestId("flow-item-timeline")).toBeNull());
    expect(invoker).toHaveFocus();
  });

  it("FlowItemTimeline_should_CapAt50SpansAndSayHowManyExist_When_MoreSpansThanShown", async () => {
    const user = userEvent.setup();
    mockClient.getItemFlowTimeline.mockResolvedValue(timeline(50, 87));
    render(<Harness />);

    await user.click(screen.getByRole("button", { name: /View events/ }));
    const dialog = await screen.findByTestId("flow-item-timeline");

    expect(await within(dialog).findAllByRole("listitem")).toHaveLength(50);
    expect(dialog).toHaveTextContent("Showing 50 of 87 spans");
  });

  it("FlowItemTimeline_should_ShowBusySkeleton_When_CallPending", async () => {
    const user = userEvent.setup();
    mockClient.getItemFlowTimeline.mockReturnValue(new Promise(() => {}));
    render(<Harness />);

    await user.click(screen.getByRole("button", { name: /View events/ }));

    expect(await screen.findByTestId("flow-item-timeline")).toHaveAttribute("aria-busy", "true");
    expect(screen.getByTestId("flow-dialog-skeleton")).toHaveAttribute("aria-busy", "true");
    await act(async () => {});
  });

  it("FlowItemTimeline_should_CloseFromCloseButton_When_Clicked", async () => {
    const user = userEvent.setup();
    mockClient.getItemFlowTimeline.mockResolvedValue(timeline(1));
    render(<Harness />);
    await user.click(screen.getByRole("button", { name: /View events/ }));
    const dialog = await screen.findByTestId("flow-item-timeline");

    await user.click(within(dialog).getByRole("button", { name: "Close" }));

    await waitFor(() => expect(screen.queryByTestId("flow-item-timeline")).toBeNull());
  });
});
