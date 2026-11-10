import { fireEvent, screen, waitFor } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { CompareFlowWindowsResponseSchema, GetFlowSummaryResponseSchema } from "@/gen/session/v1/backlog_flow_pb";
import { FlowPage } from "../FlowPage";
import { COMPARE_ALL_TIME_HINT } from "../FlowHeader";
import { AS_OF, location, makeSummary, renderAt } from "./flowFixtures";

const mockClient = { getFlowSummary: jest.fn(), compareFlowWindows: jest.fn() };

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => mockClient,
}));

const compareRadio = () => screen.getByRole("radio", { name: "previous period" });

beforeEach(() => {
  mockClient.getFlowSummary.mockReset().mockResolvedValue(create(GetFlowSummaryResponseSchema, { summary: makeSummary() }));
  mockClient.compareFlowWindows.mockReset().mockResolvedValue(create(CompareFlowWindowsResponseSchema, {}));
});

describe("FlowHeader", () => {
  it("FlowHeader_should_SupportAllTimeRadioRefreshJumpListAndWiden_When_Used", async () => {
    mockClient.getFlowSummary.mockResolvedValue(
      create(GetFlowSummaryResponseSchema, { summary: makeSummary({ meta: { asOf: timestampFromDate(AS_OF), slowPath: true } }) }),
    );
    await renderAt("/backlog/flow", <FlowPage />);

    fireEvent.click(screen.getByRole("radio", { name: "All time" }));
    expect(location()).toBe("/backlog/flow?window=all");
    await waitFor(() => expect(mockClient.getFlowSummary).toHaveBeenLastCalledWith(expect.objectContaining({ allTime: true, windowDays: 0 })));
    expect(await screen.findByText(/slow path/)).toBeInTheDocument();

    expect(screen.getByRole("radiogroup", { name: "Window" })).toBeInTheDocument();
    expect(screen.getByRole("radiogroup", { name: "Compare" })).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "Jump to section" })).toBeInTheDocument();
  });

  it("FlowHeader_should_DisableCompareAndResetToNone_When_AllTimeSelected", async () => {
    await renderAt("/backlog/flow?window=30&compare=prev", <FlowPage />);
    expect(compareRadio()).toHaveAttribute("aria-checked", "true");
    await waitFor(() => expect(mockClient.compareFlowWindows).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByRole("radio", { name: "All time" }));

    expect(location()).toBe("/backlog/flow?window=all");
    expect(compareRadio()).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("radio", { name: "none" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByText(COMPARE_ALL_TIME_HINT)).toBeInTheDocument();
    await waitFor(() => expect(mockClient.getFlowSummary).toHaveBeenCalledTimes(2));
    expect(mockClient.compareFlowWindows).toHaveBeenCalledTimes(1);

    fireEvent.click(compareRadio());
    expect(location()).toBe("/backlog/flow?window=all");

    fireEvent.click(screen.getByRole("radio", { name: "30d" }));
    expect(location()).toBe("/backlog/flow?window=30");
    expect(screen.getByRole("radio", { name: "none" })).toHaveAttribute("aria-checked", "true");
  });

  it("FlowHeader_should_DropCompareFromUrl_When_LoadedWithAllTimeAndCompare", async () => {
    await renderAt("/backlog/flow?window=all&compare=prev", <FlowPage />);

    await waitFor(() => expect(location()).toBe("/backlog/flow?window=all"));
    expect(screen.getByText(COMPARE_ALL_TIME_HINT)).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "none" })).toHaveAttribute("aria-checked", "true");
    expect(mockClient.compareFlowWindows).not.toHaveBeenCalled();
  });

  it("FlowHeader_should_SelectPreviousPeriodAndRequestComparison_When_Clicked", async () => {
    await renderAt("/backlog/flow?window=7", <FlowPage />);

    fireEvent.click(compareRadio());

    expect(location()).toBe("/backlog/flow?window=7&compare=prev");
    await waitFor(() => expect(mockClient.compareFlowWindows).toHaveBeenCalledWith(expect.objectContaining({ aDays: 7, bDays: 7, bOffsetDays: 0 })));
  });
});
