import { create } from "@bufbuild/protobuf";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { CompareFlowWindowsResponseSchema, type FlowSummary } from "@/gen/session/v1/backlog_flow_pb";
import { makeSummary } from "@/app/backlog/flow/__tests__/flowFixtures";
import { FlowTiles } from "./FlowTiles";

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));

const onDrill = jest.fn();

function summaryWith(tiles: object = {}, review: object = {}): FlowSummary {
  return makeSummary({
    tiles: {
      meta: { status: "ok" },
      merged: { value: 9, drillKey: "tiles.merged" },
      firstPassYield: { num: 32, den: 105, ratio: 32 / 105, drillKey: "fpy.first_pass" },
      leadTime: { n: 30, p50Ms: BigInt(Math.round(3.1 * 86_400_000)), p90Ms: BigInt(8 * 86_400_000), drillKey: "tiles.lead_time" },
      ...tiles,
    },
    review: {
      meta: { status: "ok" },
      rawFail: { num: 186, den: 528, ratio: 186 / 528, drillKey: "review.raw_fail" },
      adjustedFail: { num: 88, den: 355, ratio: 88 / 355, drillKey: "review.adjusted_fail" },
      ...review,
    },
  });
}

const compareWith = (metrics: object[]) => create(CompareFlowWindowsResponseSchema, { metrics } as never);
const tile = (name: string) => screen.getByTestId(`flow-tile-${name}`);

beforeEach(() => onDrill.mockReset());

describe("FlowTiles", () => {
  it("FlowTiles_should_ShowRatioWithCountFirstAndDrill_When_FpyPresent", () => {
    render(<FlowTiles summary={summaryWith()} comparison={null} onDrill={onDrill} />);

    expect(tile("fpy")).toHaveTextContent("32/105 (30.5%)");
    fireEvent.click(within(tile("fpy")).getByRole("button", { name: /32\/105/ }));
    expect(onDrill).toHaveBeenCalledWith("fpy.first_pass");
  });

  it("FlowTiles_should_GreyPercentAndLabelLowSample_When_DenominatorUnder5", () => {
    render(
      <FlowTiles
        summary={summaryWith({ firstPassYield: { num: 1, den: 3, ratio: 1 / 3, lowSample: true, drillKey: "fpy.first_pass" } })}
        comparison={null}
        onDrill={onDrill}
      />,
    );

    expect(tile("fpy")).toHaveTextContent("1/3");
    expect(tile("fpy")).toHaveTextContent("low sample");
    expect(within(tile("fpy")).getByText("(33.3%)")).toHaveAttribute("data-low-sample", "true");
  });

  it("FlowTiles_should_LeadWithAdjustedRateAndShowRawSecondary_When_Fixture528", () => {
    render(<FlowTiles summary={summaryWith()} comparison={null} onDrill={onDrill} />);

    const review = tile("review-fail");
    expect(review).toHaveTextContent("88/355 (24.8%)");
    expect(review).toHaveTextContent("FAIL, judgment verdicts only");
    expect(review).toHaveTextContent("raw 186/528 (35.2%)");
    fireEvent.click(within(review).getByRole("button", { name: /raw 186\/528/ }));
    expect(onDrill).toHaveBeenCalledWith("review.raw_fail");
    fireEvent.click(within(review).getByRole("button", { name: /88\/355/ }));
    expect(onDrill).toHaveBeenCalledWith("review.adjusted_fail");
  });

  it("FlowTiles_should_ShowNaNoDataInPreviousPeriod_When_CompareEmpty", () => {
    const comparison = compareWith([{ name: "merged_items", unit: "items", state: "ok", a: 0, b: 9, nA: 0, nB: 9 }]);
    render(<FlowTiles summary={summaryWith()} comparison={comparison} onDrill={onDrill} />);

    expect(tile("throughput")).toHaveTextContent("n/a (no data in previous period)");
    expect(tile("throughput")).not.toHaveTextContent(/NaN|Infinity/);
  });

  it("FlowTiles_should_ShowLowSampleNaWithRawValuesAndNoVerdictWord_When_Incomparable", () => {
    const comparison = compareWith([
      {
        name: "first_pass_yield",
        unit: "ratio",
        state: "incomparable",
        reason: "low sample (N_A=8, N_B=26, need 20)",
        a: 0.25,
        b: 0.31,
        nA: 8,
        nB: 26,
      },
    ]);
    render(<FlowTiles summary={summaryWith()} comparison={comparison} onDrill={onDrill} />);

    const fpy = tile("fpy");
    expect(fpy).toHaveTextContent("n/a (low sample: 8 vs 26 items, need 20)");
    expect(fpy).toHaveTextContent("32/105 (30.5%)");
    expect(fpy).not.toHaveTextContent(/better|worse/);
    expect(fpy).not.toHaveTextContent(/NaN|Infinity/);
  });

  it("FlowTiles_should_ShowArrowSignedTextAndWord_When_DeltaMeaningful", () => {
    const comparison = compareWith([
      { name: "merged_items", unit: "items", state: "ok", a: 7, b: 9, delta: 2, nA: 7, nB: 9 },
      { name: "lead_time_p50_hours", unit: "hours", state: "ok", a: 82, b: 74, delta: -9.6, nA: 20, nB: 30 },
    ]);
    render(<FlowTiles summary={summaryWith()} comparison={comparison} onDrill={onDrill} />);

    expect(tile("throughput")).toHaveTextContent("up +2");
    expect(tile("throughput")).toHaveTextContent("better");
    expect(tile("lead-time")).toHaveTextContent("down -0.4d");
    expect(tile("lead-time")).toHaveTextContent("better");
  });

  it("FlowTiles_should_ShowNoDataNotZeroPercent_When_DenominatorZero", () => {
    const summary = summaryWith({ firstPassYield: { num: 0, den: 0 } });
    render(<FlowTiles summary={summary} comparison={null} onDrill={onDrill} />);

    expect(tile("fpy")).toHaveTextContent("no data");
    expect(tile("fpy")).not.toHaveTextContent("0%");
  });

  it("FlowTiles_should_LinkOpenStuckToUnfinished_When_Rendered", () => {
    render(<FlowTiles summary={summaryWith()} comparison={null} onDrill={onDrill} />);

    const link = within(tile("open-stuck")).getByRole("link");
    expect(link).toHaveAttribute("href", "/unfinished");
    expect(link).toHaveTextContent("opens Unfinished page");
    expect(tile("open-stuck")).toHaveTextContent("8");
  });

  it("FlowTiles_should_RenderLabelledGroup_When_Rendered", () => {
    render(<FlowTiles summary={summaryWith()} comparison={null} onDrill={onDrill} />);

    expect(screen.getByRole("group", { name: "Health summary" })).toBeInTheDocument();
  });
});
