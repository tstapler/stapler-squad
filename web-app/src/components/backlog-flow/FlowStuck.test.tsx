import { create } from "@bufbuild/protobuf";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { FlowStuckSectionSchema, type FlowStuckSection } from "@/gen/session/v1/backlog_flow_pb";
import { FlowStuck } from "./FlowStuck";

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));

const onDrill = jest.fn();
const HOUR = 3_600_000;
const CAPTURE = new Date(Date.UTC(2026, 9, 20, 12));

const stuck = (init: object = {}): FlowStuckSection =>
  create(FlowStuckSectionSchema, {
    meta: { status: "ok" },
    open: 8,
    reasons: [
      {
        reason: "needs_input",
        open: 3,
        distinctItems: { value: 5, drillKey: "stuck.needs_input" },
        duration: { n: 7, p50Ms: BigInt(4 * HOUR), p90Ms: BigInt(50 * HOUR), drillKey: "stuck.needs_input" },
      },
    ],
    ...init,
  } as never);

const view = (estimated: string[], section = stuck()) =>
  render(<FlowStuck stuck={section} estimated={estimated} captureSince={CAPTURE} onDrill={onDrill} />);

beforeEach(() => onDrill.mockReset());

describe("FlowStuck", () => {
  it("FlowStuck_should_ShowLowerBoundText_When_StuckHistoryLowerBoundEstimated", () => {
    view(["stuck_history:lower_bound"]);

    expect(screen.getByTestId("flow-stuck")).toHaveTextContent("history before 2026-10-20 is a lower bound");
    expect(screen.getAllByText(/^~/).length).toBeGreaterThan(0);
  });

  it("FlowStuck_should_NotClaimLowerBound_When_NotEstimated", () => {
    view([]);

    expect(screen.getByTestId("flow-stuck")).not.toHaveTextContent("lower bound");
  });

  it("FlowStuck_should_LinkCurrentCountAndDrillHistory_When_ReasonRowShown", () => {
    view([]);

    const row = screen.getByRole("row", { name: /needs_input/ });
    expect(within(row).getByRole("link", { name: /3/ })).toHaveAttribute("href", "/unfinished");
    fireEvent.click(within(row).getByRole("button", { name: /7/ }));
    expect(onDrill).toHaveBeenCalledWith("stuck.needs_input");
    expect(row).toHaveTextContent("median 4h");
    expect(row).toHaveTextContent("p90 2.1d");
  });

  it("FlowStuck_should_SayNothingStuckWithHistory_When_NoneOpen", () => {
    view([], stuck({ open: 0 }));

    expect(screen.getByTestId("flow-stuck")).toHaveTextContent("Nothing stuck now; 7 episodes historically");
  });
});
