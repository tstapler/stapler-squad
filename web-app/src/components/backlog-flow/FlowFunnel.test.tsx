import { create } from "@bufbuild/protobuf";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { FlowFunnelSectionSchema, type FlowFunnelSection } from "@/gen/session/v1/backlog_flow_pb";
import { FlowFunnel } from "./FlowFunnel";

const onDrill = jest.fn();
const HOUR = 3_600_000;

const stage = (status: string, entered: number, advanced: number, backEdges = 0, dwell = { n: 10, p50Ms: BigInt(2 * HOUR), p90Ms: BigInt(24 * HOUR) }) => ({
  status,
  entered: { value: entered, drillKey: `funnel.stage.${status}` },
  advanced,
  backEdges,
  dwell,
});

const funnel = (init: object): FlowFunnelSection => create(FlowFunnelSectionSchema, { meta: { status: "ok" }, ...init } as never);

beforeEach(() => onDrill.mockReset());

describe("FlowFunnel", () => {
  it("FlowFunnel_should_ShowRealPrimaryAndExcludedRawInLabel_When_ThrashEpisodesPresent", () => {
    const section = funnel({
      stages: [stage("in_progress", 141, 105), stage("pr_pending", 100, 90, 96)],
      thrash: { n: 7 },
    });
    render(<FlowFunnel funnel={section} collapsedEvents={4341} onDrill={onDrill} />);

    const row = screen.getByTestId("flow-funnel-stage-pr_pending");
    expect(row).toHaveTextContent("pr_pending -> earlier stage: 96 real reopens (4,341 raw events in 7 thrash episodes excluded)");
    expect(row).not.toHaveTextContent("4,437");
    fireEvent.click(within(row).getByRole("button", { name: /96 real reopens/ }));
    expect(onDrill).toHaveBeenCalledWith("funnel.stage.pr_pending");
  });

  it("FlowFunnel_should_NameBackEdgeTargetsAndDrillEach_When_TargetsPresent", () => {
    const section = funnel({
      stages: [
        {
          ...stage("review", 100, 80, 440, undefined),
          advancedDrillKey: "funnel.advanced.review",
          backEdgeTargets: [
            { toStage: "in_progress", count: 426, drillKey: "funnel.back_edge.review.in_progress" },
            { toStage: "ready", count: 14, drillKey: "funnel.back_edge.review.ready" },
          ],
        },
      ],
    });
    render(<FlowFunnel funnel={section} collapsedEvents={0} onDrill={onDrill} />);

    const row = screen.getByTestId("flow-funnel-stage-review");
    expect(row).toHaveTextContent("review -> in_progress 426");
    expect(row).toHaveTextContent("review -> ready 14");
    expect(row).not.toHaveTextContent("earlier stage");
    fireEvent.click(within(row).getByRole("button", { name: /review -> in_progress 426/ }));
    expect(onDrill).toHaveBeenCalledWith("funnel.back_edge.review.in_progress");
    fireEvent.click(within(row).getByRole("button", { name: /advanced 80/ }));
    expect(onDrill).toHaveBeenCalledWith("funnel.advanced.review");
  });

  it("FlowFunnel_should_RenderStagesFromResponseAndOtherRow_When_CustomStageOrder", () => {
    const section = funnel({
      stages: [stage("idea", 158, 150), stage("triage", 150, 141), stage("ready", 141, 100), stage("done", 100, 0)],
      other: 3,
    });
    render(<FlowFunnel funnel={section} collapsedEvents={0} onDrill={onDrill} />);

    const items = within(screen.getByRole("list", { name: "Funnel stages" })).getAllByRole("listitem");
    expect(items.map((li) => li.getAttribute("data-testid"))).toEqual([
      "flow-funnel-stage-idea",
      "flow-funnel-stage-triage",
      "flow-funnel-stage-ready",
      "flow-funnel-stage-done",
      "flow-funnel-other",
    ]);
    expect(screen.getByTestId("flow-funnel-stage-triage")).toHaveTextContent("Stage 2 of 4: triage");
    expect(screen.getByTestId("flow-funnel-other")).toHaveTextContent("Other stages: 3");
    expect(screen.queryByTestId("flow-funnel-stage-refining")).not.toBeInTheDocument();
  });

  it("FlowFunnel_should_ShowEnteredAdvancedDwellAndMirrorInHiddenTable_When_StageHasData", () => {
    const section = funnel({ stages: [stage("in_progress", 141, 105, 0, { n: 5, p50Ms: BigInt(26 * HOUR), p90Ms: BigInt(96 * HOUR) })] });
    render(<FlowFunnel funnel={section} collapsedEvents={0} onDrill={onDrill} />);

    const row = screen.getByTestId("flow-funnel-stage-in_progress");
    expect(row).toHaveTextContent("entered 141");
    expect(row).toHaveTextContent("advanced 105");
    expect(row).toHaveTextContent("median 1.1d");
    expect(row).toHaveTextContent("p90 4d");
    const table = screen.getByRole("table", { name: /Funnel data/ });
    expect(within(table).getByRole("row", { name: /in_progress/ })).toHaveTextContent("141");
  });

  it("FlowFunnel_should_SayNoItemsEnteredAndNaDwell_When_StageEmpty", () => {
    const section = funnel({ stages: [stage("review", 0, 0, 0, { n: 0, p50Ms: 0n, p90Ms: 0n })] });
    render(<FlowFunnel funnel={section} collapsedEvents={0} onDrill={onDrill} />);

    const row = screen.getByTestId("flow-funnel-stage-review");
    expect(row).toHaveTextContent("no items entered");
    expect(row).toHaveTextContent("dwell n/a");
  });
});
