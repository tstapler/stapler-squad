import { create } from "@bufbuild/protobuf";
import { render, screen } from "@testing-library/react";
import { FlowMetaSchema, FlowNotesSectionSchema, type FlowMeta, type FlowNotesSection } from "@/gen/session/v1/backlog_flow_pb";
import { FlowDataNotes } from "./FlowDataNotes";

const meta = (init: object = {}): FlowMeta =>
  create(FlowMetaSchema, {
    rowCounts: { events: 87_000, verdicts: 528, stuckIntervals: 40, items: 210 },
    queryMs: 412n,
    estimated: ["stuck_history:lower_bound", "deadlock_candidates:heuristic"],
    estimatedLegend: { "stuck_history:lower_bound": "Stuck history starts at capture", "deadlock_candidates:heuristic": "Streaks, not verdicts" },
    ...init,
  } as never);

const notes = (init: object = {}): FlowNotesSection =>
  create(FlowNotesSectionSchema, {
    meta: { status: "ok" },
    rawEvents: 87_000,
    realSpans: 900,
    collapsedEvents: 4_341,
    notes: ["self-loop events were dropped before aggregation"],
    ...init,
  } as never);

describe("FlowDataNotes", () => {
  it("FlowDataNotes_should_ListEstimatedWithLegendAndCounts_When_Opened", () => {
    render(<FlowDataNotes notes={notes()} meta={meta()} />);

    const body = screen.getByTestId("flow-data-notes");
    expect(body).toHaveTextContent("stuck_history:lower_bound");
    expect(body).toHaveTextContent("Stuck history starts at capture");
    expect(body).toHaveTextContent("Streaks, not verdicts");
    expect(body).toHaveTextContent("87,000 events");
    expect(body).toHaveTextContent("528 verdicts");
    expect(body).toHaveTextContent("Query 412 ms");
    expect(body).toHaveTextContent("4,341 raw events collapsed into thrash episodes");
    expect(body).toHaveTextContent("Archived excluded from stages, included in throughput");
    expect(body).toHaveTextContent("self-loop events were dropped before aggregation");
    expect(body).toHaveTextContent("15 minutes");
  });

  it("FlowDataNotes_should_ShowMethodologyUnavailable_When_MetaMissing", () => {
    render(<FlowDataNotes notes={notes()} meta={undefined} />);

    expect(screen.getByText("Methodology details unavailable")).toBeInTheDocument();
    expect(screen.queryByTestId("flow-data-notes")).not.toBeInTheDocument();
  });

  it("FlowDataNotes_should_NameEveryEstimatedCodeEvenWithoutLegend_When_LegendMissing", () => {
    render(<FlowDataNotes notes={notes()} meta={meta({ estimatedLegend: {} })} />);

    expect(screen.getByTestId("flow-data-notes")).toHaveTextContent("deadlock_candidates:heuristic");
  });

  it("FlowDataNotes_should_ShowQualityCounts_When_AnyNonZero", () => {
    render(<FlowDataNotes notes={notes({ quality: { selfLoops: 3, unparseable: 0, negativeGaps: 1, unknownSpans: 0 } })} meta={meta()} />);

    expect(screen.getByTestId("flow-data-notes")).toHaveTextContent("3 self-loops");
    expect(screen.getByTestId("flow-data-notes")).toHaveTextContent("1 negative gap");
  });
});
