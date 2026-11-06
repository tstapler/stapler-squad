import { create } from "@bufbuild/protobuf";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FlowCausesSectionSchema, type FlowCausesSection } from "@/gen/session/v1/backlog_flow_pb";
import { FlowCauses } from "./FlowCauses";

const onDrill = jest.fn();

const row = (cause: string, value: number) => ({ cause, count: { value, drillKey: `rework.${cause}` } });

function causes(init: object = {}): FlowCausesSection {
  return create(FlowCausesSectionSchema, {
    meta: { status: "ok" },
    rework: [row("bounce", 28), row("review_fail", 398), row("pr_fix", 103)],
    mergedCount: 105,
    ...init,
  } as never);
}

const bodyRows = () => within(screen.getByRole("table")).getAllByRole("row").slice(1);

beforeEach(() => onDrill.mockReset());

describe("FlowCauses", () => {
  it("FlowCauses_should_RankByCountAndDrillEachCount_When_Rendered", () => {
    render(<FlowCauses causes={causes()} estimated={[]} onDrill={onDrill} />);

    const rows = bodyRows();
    expect(rows.map((r) => r.textContent)).toEqual([expect.stringContaining("Review FAIL rework"), expect.stringContaining("PR fix"), expect.stringContaining("Bounce")]);
    fireEvent.click(within(rows[0]).getByRole("button", { name: "398" }));
    expect(onDrill).toHaveBeenCalledWith("rework.review_fail");
  });

  it("FlowCauses_should_StatePrFixCauseGap_When_TriggerCaptureUnavailable", () => {
    render(<FlowCauses causes={causes()} estimated={["pr_fix_cause:unavailable"]} onDrill={onDrill} />);

    expect(screen.getByText(/PR fix \(CI\/review\/conflict not separable/)).toBeInTheDocument();
  });

  it("FlowCauses_should_SetAriaSortAndToggleWithKeyboard_When_HeaderActivated", async () => {
    const user = userEvent.setup();
    render(<FlowCauses causes={causes()} estimated={[]} onDrill={onDrill} />);
    const cause = screen.getByRole("columnheader", { name: /Cause/ });
    const count = screen.getByRole("columnheader", { name: /Count/ });

    expect(count).toHaveAttribute("aria-sort", "descending");
    expect(cause).not.toHaveAttribute("aria-sort", "ascending");
    expect(cause).not.toHaveAttribute("aria-sort", "descending");

    await user.tab();
    await user.click(within(cause).getByRole("button"));
    expect(cause).toHaveAttribute("aria-sort", "ascending");
    expect(count).not.toHaveAttribute("aria-sort", "descending");
    expect(bodyRows()[0]).toHaveTextContent("Bounce");

    within(cause).getByRole("button").focus();
    await user.keyboard("{Enter}");
    expect(cause).toHaveAttribute("aria-sort", "descending");
    await user.keyboard(" ");
    expect(cause).toHaveAttribute("aria-sort", "ascending");

    await user.click(within(count).getByRole("button"));
    expect(count).toHaveAttribute("aria-sort", "descending");
  });

  it("Tables_should_HaveCaptionScopeAndLabelledScrollRegion_When_CausesRendered", () => {
    render(<FlowCauses causes={causes()} estimated={[]} onDrill={onDrill} />);

    expect(screen.getByRole("table", { name: /Rework causes/ })).toBeInTheDocument();
    screen.getAllByRole("columnheader").forEach((th) => expect(th).toHaveAttribute("scope", "col"));
    const region = screen.getByRole("region", { name: /Rework causes/ });
    expect(region).toHaveAttribute("tabindex", "0");
  });

  it("FlowCauses_should_SayNoRework_When_AllCountsZero", () => {
    render(<FlowCauses causes={causes({ rework: [row("review_fail", 0)], mergedCount: 12 })} estimated={[]} onDrill={onDrill} />);

    expect(screen.getByText(/No rework in this window/)).toHaveTextContent("12 merged items");
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });
});
