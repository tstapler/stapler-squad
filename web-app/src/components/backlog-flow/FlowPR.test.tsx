import { create } from "@bufbuild/protobuf";
import { fireEvent, render, screen } from "@testing-library/react";
import { FlowCausesSectionSchema, FlowPRSectionSchema, type FlowCausesSection, type FlowPRSection } from "@/gen/session/v1/backlog_flow_pb";
import { FlowPR } from "./FlowPR";

const onDrill = jest.fn();
const HOUR = 3_600_000;
const CAPTURE = new Date(Date.UTC(2026, 9, 20, 12));

const pr = (init: object = {}): FlowPRSection =>
  create(FlowPRSectionSchema, {
    meta: { status: "ok" },
    fixDwell: { n: 12, p50Ms: BigInt(5 * HOUR), p90Ms: BigInt(30 * HOUR), drillKey: "pr.fix_dwell" },
    cycles: { value: 103, drillKey: "pr.fix_dwell" },
    ...init,
  } as never);

const causes = (triggers: object[] = []): FlowCausesSection =>
  create(FlowCausesSectionSchema, {
    meta: { status: "ok" },
    rework: [{ cause: "pr_fix", count: { value: 103, drillKey: "rework.pr_fix" } }],
    prFixTriggers: triggers,
  } as never);

const render3 = (estimated: string[], triggers: object[] = [], capture: Date | null = CAPTURE, section = pr()) =>
  render(<FlowPR pr={section} causes={causes(triggers)} estimated={estimated} captureSince={capture} onDrill={onDrill} />);

beforeEach(() => onDrill.mockReset());

describe("FlowPR estimated values", () => {
  it("EstimatedValue_should_RenderMarkerAndSrOnlyText_When_MetricEstimated", () => {
    render3(["pr_fix_dwell:combined"]);

    const marker = screen.getByTitle("Combined PR-fix dwell; CI and conflict are not separable");
    expect(marker).toHaveTextContent(/^~/);
    const description = document.getElementById(marker.getAttribute("aria-describedby") as string);
    expect(description).toHaveTextContent("Combined PR-fix dwell; CI and conflict are not separable");
  });
});

describe("FlowPR", () => {
  it("FlowPR_should_ShowCombinedDwellAndNotYetRecording_When_NoCaptureAndNoOutcomes", () => {
    render3(["pr_fix_dwell:combined", "pr_fix_cause:unavailable"], [], null);

    const section = screen.getByTestId("flow-pr");
    expect(section).toHaveTextContent("median 5h, p90 1.3d, N 12");
    expect(section).toHaveTextContent("CI and conflict dwell are not separable before");
    expect(section).toHaveTextContent("PR fix (CI/review/conflict not separable)");
    expect(section).toHaveTextContent("CI vs conflict split not recorded");
    expect(screen.getByTestId("flow-pr-outcomes")).toHaveTextContent("Outcome tracking not yet recording");
    fireEvent.click(screen.getByRole("button", { name: /median 5h/ }));
    expect(onDrill).toHaveBeenCalledWith("pr.fix_dwell");
  });

  it("FlowPR_should_ShowNotYetRecordingNotZeros_When_NoOutcomeCapture", () => {
    render3(["pr_fix_cause:unavailable"]);

    expect(screen.getByTestId("flow-pr-outcomes").textContent).toBe("Outcome tracking not yet recording");
    expect(screen.getByTestId("flow-pr-outcomes")).not.toHaveTextContent(/\d/);
  });

  it("FlowPR_should_NameCaptureDate_When_CaptureSinceKnown", () => {
    render3(["pr_fix_dwell:combined", "pr_fix_cause:unavailable"]);

    expect(screen.getByTestId("flow-pr")).toHaveTextContent("CI vs conflict split not recorded before 2026-10-20");
    expect(screen.getByTestId("flow-pr")).toHaveTextContent("CI and conflict dwell are not separable before 2026-10-20");
  });

  it("FlowPR_should_SplitByTriggerIncludingMixed_When_CaptureExists", () => {
    const triggers = ["ci", "conflict", "mixed", "review", "feedback", "unknown"].map((trigger, index) => ({
      trigger,
      count: index + 1,
      drillKey: `rework.pr_fix`,
    }));
    render3(["pr_fix_cause:since_capture", "pr_fix_dwell_split:since_capture"], triggers);

    const split = screen.getByTestId("flow-pr-triggers");
    ["ci", "conflict", "mixed", "review", "feedback", "unknown"].forEach((name) => expect(split).toHaveTextContent(name));
    expect(split).toHaveTextContent("events since 2026-10-20");
    expect(screen.getByTestId("flow-pr")).not.toHaveTextContent("not separable");
  });

  it("FlowPR_should_ShowOutcomeCountsAsEstimated_When_Recording", () => {
    const outcomes = { recording: true, steered: 5, respawned: 2, skippedActive: 1, resolvedWithin24h: 3 };
    render3(["pr_fix_outcome:since_capture"], [], CAPTURE, pr({ outcomes }));

    const box = screen.getByTestId("flow-pr-outcomes");
    expect(box).toHaveTextContent("steered: ~5");
    expect(box).toHaveTextContent("respawned: ~2");
    expect(box).toHaveTextContent("skipped (session active): ~1");
    expect(box).toHaveTextContent("resolved within 24h: ~3");
    expect(box).not.toHaveTextContent("not yet recording");
  });

  it("FlowPR_should_ShowTriggerDwellSplit_When_DwellSplitEstimated", () => {
    const triggerDwell = [
      { trigger: "ci", cycles: { value: 4 }, dwell: { n: 4, p50Ms: BigInt(2 * HOUR), p90Ms: BigInt(4 * HOUR), drillKey: "pr.fix_dwell" } },
      { trigger: "conflict", cycles: { value: 2 }, dwell: { n: 2, p50Ms: BigInt(6 * HOUR), p90Ms: BigInt(8 * HOUR), drillKey: "pr.fix_dwell" } },
    ];
    render3(["pr_fix_dwell:combined", "pr_fix_dwell_split:since_capture"], [], CAPTURE, pr({ triggerDwell }));

    const split = screen.getByTestId("flow-pr-trigger-dwell");
    expect(split).toHaveTextContent("ci: median 2h, p90 4h, N 4");
    expect(split).toHaveTextContent("conflict: median 6h, p90 8h, N 2");
  });

  it("FlowPR_should_HideTriggerDwell_When_SplitNotEstimated", () => {
    render3(["pr_fix_dwell:combined"]);

    expect(screen.queryByTestId("flow-pr-trigger-dwell")).not.toBeInTheDocument();
  });

  it("FlowPR_should_SayNoPrFixes_When_NothingInWindow", () => {
    render3([], [], CAPTURE, pr({ cycles: { value: 0 }, fixDwell: { n: 0 } }));

    expect(screen.getByText("No PR fixes in this window")).toBeInTheDocument();
  });
});
