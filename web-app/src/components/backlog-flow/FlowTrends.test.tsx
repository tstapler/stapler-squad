import { create } from "@bufbuild/protobuf";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { FlowTrendsSectionSchema, type FlowTrendsSection } from "@/gen/session/v1/backlog_flow_pb";
import { makeSummary, makeTrends, weekKey } from "@/app/backlog/flow/__tests__/flowFixtures";
import { FlowSectionBody } from "./FlowSectionBody";
import { FlowVerdictMix } from "./FlowVerdictMix";
import { CauseSparkline, ThroughputSparkline } from "./FlowTrends";

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));

const trends = (): FlowTrendsSection => create(FlowTrendsSectionSchema, makeTrends() as never);
const onDrill = jest.fn();

describe("FlowVerdictMix", () => {
  it("FlowVerdictMix_should_RenderStackedChartWithTakeawayAndTable_When_FiveWeeklyBuckets", () => {
    render(<FlowVerdictMix trends={trends()} />);

    const mix = screen.getByTestId("flow-verdict-mix");
    expect(within(mix).getByRole("img")).toHaveAttribute("aria-label", "FAIL share fell from 41% to 28% over 5 weeks");
    for (const outcome of ["PASS", "FAIL", "PARTIAL", "UNVERIFIABLE"]) {
      expect(within(mix).getAllByText(outcome).length).toBeGreaterThan(0);
    }
    const table = within(mix).getByTestId("flow-chart-table");
    expect(within(table).getAllByRole("row")).toHaveLength(6);
    expect(mix.querySelectorAll("svg[data-encoding]")).toHaveLength(4);
    expect(new Set([...mix.querySelectorAll("svg[data-encoding]")].map((swatch) => swatch.getAttribute("data-encoding"))).size).toBe(4);
  });

  it("FlowVerdictMix_should_RenderEmptyWeekAsLabelledZero_When_WeekHasNoVerdicts", () => {
    render(<FlowVerdictMix trends={trends()} />);

    const emptyWeek = within(screen.getByTestId("flow-chart-table")).getByRole("row", { name: /2026-10-19/ });
    expect(within(emptyWeek).getAllByRole("cell").map((cell) => cell.textContent)).toEqual(["0", "0", "0", "0"]);
  });

  it("FlowVerdictMix_should_RenderNothing_When_NoTrendBuckets", () => {
    const { container } = render(<FlowVerdictMix trends={undefined} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe("Sparklines", () => {
  it("ThroughputSparkline_should_PrintFirstAndLastWeeklyValues_When_TrendsPresent", () => {
    render(<ThroughputSparkline trends={trends()} />);

    const spark = screen.getByTestId("flow-sparkline-throughput");
    expect(spark).toHaveTextContent("first week 4, last week 9");
    expect(within(spark).getByRole("img")).toHaveAttribute("aria-label", expect.stringContaining("4 in the week of 2026-10-05"));
  });

  it("CauseSparkline_should_UseTheCauseSeries_When_ReworkBucketsPresent", () => {
    render(<CauseSparkline trends={trends()} cause="review_fail" label="Review FAIL rework" />);

    expect(screen.getByTestId("flow-sparkline-review_fail")).toHaveTextContent("first week 30, last week 12");
  });

  it("ThroughputSparkline_should_DrillIntoThatWeek_When_WeekRowButtonClicked", () => {
    const drill = jest.fn();
    render(<ThroughputSparkline trends={trends()} onDrill={drill} />);

    fireEvent.click(within(screen.getByTestId("flow-chart-table")).getByRole("button", { name: "2026-10-12" }));

    expect(drill).toHaveBeenCalledWith(weekKey("merged", 1));
  });

  it("ThroughputSparkline_should_DrillIntoThatWeek_When_PointClicked", () => {
    const drill = jest.fn();
    render(<ThroughputSparkline trends={trends()} onDrill={drill} />);

    fireEvent.click(screen.getAllByTestId("flow-chart-point")[3]);

    expect(drill).toHaveBeenCalledWith(weekKey("merged", 3));
  });

  it("CauseSparkline_should_DrillWithTheCauseWeekKey_When_WeekRowButtonClicked", () => {
    const drill = jest.fn();
    render(<CauseSparkline trends={trends()} cause="review_fail" label="Review FAIL rework" onDrill={drill} />);

    fireEvent.click(within(screen.getByTestId("flow-chart-table")).getByRole("button", { name: "2026-10-05" }));

    expect(drill).toHaveBeenCalledWith(weekKey("rework.review_fail", 0));
  });

  it("Sparklines_should_StayPlain_When_NoDrillHandlerGiven", () => {
    render(<ThroughputSparkline trends={trends()} />);

    expect(within(screen.getByTestId("flow-chart-table")).queryByRole("button")).toBeNull();
    expect(screen.queryAllByTestId("flow-chart-point")).toHaveLength(0);
  });

  it("FlowVerdictMix_should_StayNonDrillable_When_Rendered", () => {
    render(<FlowVerdictMix trends={trends()} />);

    expect(within(screen.getByTestId("flow-chart-table")).queryByRole("button")).toBeNull();
  });

  it("CauseSparkline_should_RenderNothing_When_TrendsUnavailable", () => {
    const { container } = render(<CauseSparkline trends={undefined} cause="bounce" label="Bounce" />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe("FlowChart consumers", () => {
  it("FlowChart_should_BeRenderedByAtLeastThreeConsumers_When_SectionsRenderWithTrends", () => {
    const summary = makeSummary({
      causes: { meta: { status: "ok" }, mergedCount: 10, rework: [{ cause: "review_fail", count: { value: 5, drillKey: "rework.review_fail" } }] },
      review: { meta: { status: "ok" }, total: { value: 10 }, adjustedFail: { num: 2, den: 8 }, rawFail: { num: 3, den: 10 } },
    });
    render(
      <>
        {(["tiles", "causes", "review"] as const).map((id) => (
          <FlowSectionBody key={id} id={id} summary={summary} comparison={null} onDrill={onDrill} />
        ))}
      </>,
    );

    for (const consumer of ["flow-sparkline-throughput", "flow-sparkline-review_fail", "flow-verdict-mix"]) {
      expect(within(screen.getByTestId(consumer)).getByTestId("flow-chart")).toBeInTheDocument();
    }
    expect(screen.getAllByTestId("flow-chart").length).toBeGreaterThanOrEqual(3);
  });
});
