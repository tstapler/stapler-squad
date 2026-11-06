import { create } from "@bufbuild/protobuf";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { FlowReviewSectionSchema, type FlowReviewSection as ReviewProto } from "@/gen/session/v1/backlog_flow_pb";
import { FlowReview } from "./FlowReview";

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));

const onDrill = jest.fn();
const cls = (name: string, value: number) => ({ class: name, total: { value, drillKey: `review.class.${name}` } });

function review(init: object = {}): ReviewProto {
  return create(FlowReviewSectionSchema, {
    meta: { status: "ok" },
    total: { value: 528, drillKey: "review.verdicts" },
    classes: [cls("judgment", 355), cls("no_commits", 98), cls("self_check_failed", 34), cls("reviewer_degraded", 18), cls("auto_verified", 23)],
    rawFail: { num: 186, den: 528, drillKey: "review.raw_fail" },
    adjustedFail: { num: 88, den: 355, drillKey: "review.adjusted_fail" },
    override: { num: 5, den: 150, drillKey: "review.override" },
    deadlocks: { items: [], totalCount: 0, returnedCount: 0 },
    weakFilter: "weak filter text",
    ...init,
  } as never);
}

beforeEach(() => onDrill.mockReset());

describe("FlowReview", () => {
  it("FlowReview_should_ShowRawAndAdjustedRatesWithExcludedNote_When_Fixture528", () => {
    render(<FlowReview review={review()} estimated={[]} onDrill={onDrill} />);

    const rates = screen.getByTestId("flow-review-rates");
    expect(rates).toHaveTextContent("Adjusted FAIL 88/355 (24.8%)");
    expect(rates).toHaveTextContent("Raw 186/528 (35.2%)");
    expect(rates).toHaveTextContent("98 'no committed changes' and 34 self-check failures excluded from adjusted");
    expect(rates).toHaveTextContent("18 reviewer-degraded results and 23 auto-verified passes also excluded");
    expect(rates).toHaveTextContent("Human overrides 5");
    fireEvent.click(within(rates).getByRole("button", { name: /88\/355/ }));
    expect(onDrill).toHaveBeenCalledWith("review.adjusted_fail");
    expect(rates.textContent?.indexOf("Adjusted")).toBeLessThan(rates.textContent?.indexOf("Raw") ?? -1);
  });

  it("FlowReview_should_ShowRawUnverifiableRatioAndDrill_When_Present", () => {
    render(<FlowReview review={review({ unverifiable: { num: 110, den: 528, drillKey: "review.unverifiable" } })} estimated={[]} onDrill={onDrill} />);

    const line = screen.getByTestId("flow-review-unverifiable");
    expect(line).toHaveTextContent("UNVERIFIABLE 110/528 (20.8%)");
    fireEvent.click(within(line).getByRole("button", { name: /110\/528/ }));
    expect(onDrill).toHaveBeenCalledWith("review.unverifiable");
  });

  it("FlowReview_should_OmitUnverifiableLine_When_DenominatorZero", () => {
    render(<FlowReview review={review()} estimated={[]} onDrill={onDrill} />);

    expect(screen.queryByTestId("flow-review-unverifiable")).not.toBeInTheDocument();
  });

  it("FlowReview_should_SayNoVerdicts_When_TotalZero", () => {
    render(<FlowReview review={review({ total: { value: 0 }, classes: [] })} estimated={[]} onDrill={onDrill} />);

    expect(screen.getByText("No review verdicts in this window")).toBeInTheDocument();
  });

  it("FlowReview_should_ShowDeadlockTableWithItsOwnState_When_VerdictRatesLowSample", () => {
    render(<FlowReview review={review({ adjustedFail: { num: 1, den: 3, lowSample: true } })} estimated={[]} onDrill={onDrill} />);

    expect(screen.getByTestId("flow-review-rates")).toHaveTextContent("low sample");
    expect(screen.getByRole("heading", { name: "Review-fail streaks (heuristic)" })).toBeInTheDocument();
  });
});
