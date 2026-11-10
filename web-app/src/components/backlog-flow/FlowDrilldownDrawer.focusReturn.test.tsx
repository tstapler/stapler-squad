/**
 * Focus contract of the drill-down drawer, driven through FlowPage's real number buttons so the
 * invoker wiring is under test, as BacklogItemDetail.focusReturn.test.tsx does for its modals.
 */
import { create } from "@bufbuild/protobuf";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { GetFlowSummaryResponseSchema, ListFlowDrilldownResponseSchema } from "@/gen/session/v1/backlog_flow_pb";
import { FlowPage } from "@/app/backlog/flow/FlowPage";
import { makeSummary, renderAt } from "@/app/backlog/flow/__tests__/flowFixtures";

const mockClient = { getFlowSummary: jest.fn(), compareFlowWindows: jest.fn(), getItemFlowTimeline: jest.fn(), listFlowDrilldown: jest.fn() };

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({ ...jest.requireActual("@connectrpc/connect"), createClient: () => mockClient }));

beforeEach(() => {
  const summary = makeSummary({ tiles: { meta: { status: "ok" }, merged: { value: 9, drillKey: "tiles.merged" } } });
  mockClient.getFlowSummary.mockReset().mockResolvedValue(create(GetFlowSummaryResponseSchema, { summary }));
  mockClient.compareFlowWindows.mockReset();
  mockClient.listFlowDrilldown.mockReset().mockResolvedValue(
    create(ListFlowDrilldownResponseSchema, { items: { items: [{ itemId: "i1", title: "Fix login test", status: "review", reason: "FAIL" }], totalCount: 1, returnedCount: 1 } } as never),
  );
});

describe("FlowDrilldownDrawer focus", () => {
  it("FlowDrilldownDrawer_should_TrapFocusAndReturnToInvoker_When_Escape", async () => {
    const user = userEvent.setup();
    await renderAt("/backlog/flow", <FlowPage />);
    const invoker = await screen.findByRole("button", { name: /9 merged/ });

    await user.click(invoker);
    const dialog = await screen.findByTestId("flow-drilldown-drawer");
    await within(dialog).findByText("Fix login test");
    expect(within(dialog).getByRole("heading", { name: /Merged items/ })).toHaveFocus();

    for (let i = 0; i < 12; i++) {
      await user.tab();
      expect(dialog).toContainElement(document.activeElement as HTMLElement);
    }
    await user.tab({ shift: true });
    expect(dialog).toContainElement(document.activeElement as HTMLElement);

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByTestId("flow-drilldown-drawer")).toBeNull());
    expect(invoker).toHaveFocus();
  });

  it("FlowDrilldownDrawer_should_ReturnToInvoker_When_ClosedWithCloseButton", async () => {
    const user = userEvent.setup();
    await renderAt("/backlog/flow", <FlowPage />);
    const invoker = await screen.findByRole("button", { name: /9 merged/ });

    await user.click(invoker);
    await user.click(await screen.findByRole("button", { name: "Close" }));

    await waitFor(() => expect(screen.queryByTestId("flow-drilldown-drawer")).toBeNull());
    expect(invoker).toHaveFocus();
  });
});
