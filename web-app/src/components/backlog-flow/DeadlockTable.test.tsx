import { create } from "@bufbuild/protobuf";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { FlowDeadlockPageSchema, type FlowDeadlockPage } from "@/gen/session/v1/backlog_flow_pb";
import { DeadlockTable } from "./DeadlockTable";

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));

const WEAK = "treat as a ranked review-fail streak list, not a verdict on the reviewer";

function candidate(index: number, runLength: number) {
  return {
    itemId: `item${String(index).padStart(4, "0")}abcdef`,
    title: `Item ${index}`,
    resolution: "auto_verified",
    sequence: "FRUUURURRURRRP",
    runLength,
    repeatedCriterion: 3,
    hasRepeatedCriterion: index % 2 === 0,
    diffUnchangedPairs: 0,
    agentClaimedDone: index === 1,
  };
}

function page(count: number, total = count): FlowDeadlockPage {
  const items = Array.from({ length: count }, (_, i) => candidate(i + 1, i + 1));
  return create(FlowDeadlockPageSchema, { items, totalCount: total, returnedCount: count, hasMore: total > count } as never);
}

const rows = () => within(screen.getByRole("table")).getAllByRole("row").slice(1);

describe("DeadlockTable", () => {
  it("DeadlockTable_should_ShowTopTenAndWeakFilterNote_When_MoreThanTenCandidates", () => {
    render(<DeadlockTable page={page(12, 14)} weakFilter={WEAK} estimated />);

    expect(rows()).toHaveLength(10);
    expect(rows()[0]).toHaveTextContent("Item 12");
    expect(screen.getByText(/Showing 10 of 14/)).toBeInTheDocument();
    expect(screen.getByText(new RegExp(WEAK))).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Show all" }));
    expect(rows()).toHaveLength(12);
    expect(screen.queryByRole("button", { name: "Show all" })).not.toBeInTheDocument();
  });

  it("DeadlockTable_should_ShowStreakNameAndHideAgentClaimedColumn_When_NotPromoted", () => {
    render(<DeadlockTable page={page(2)} weakFilter={WEAK} estimated />);

    expect(screen.getByRole("heading", { name: "Review-fail streaks (heuristic)" })).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: /Agent claimed done/ })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("checkbox", { name: "Show agent-claimed-done (info only)" }));
    expect(screen.getByRole("columnheader", { name: /Agent claimed done/ })).toBeInTheDocument();
  });

  it("DeadlockTable_should_UseDeadlockCandidatesHeading_When_PromotionConstantSet", () => {
    render(<DeadlockTable page={page(1)} weakFilter={WEAK} estimated promoted />);

    expect(screen.getByRole("heading", { name: "Deadlock candidates" })).toBeInTheDocument();
  });

  it("DeadlockTable_should_SpellOutSequenceAndLinkItem_When_RowRendered", () => {
    render(<DeadlockTable page={page(2)} weakFilter={WEAK} estimated />);

    const first = rows().find((r) => r.textContent?.includes("Item 2"));
    expect(first).toBeDefined();
    expect(within(first as HTMLElement).getByLabelText(/^fail, partial, unverifiable/)).toHaveTextContent("FRUUURURRURRRP");
    expect(within(first as HTMLElement).getByRole("link", { name: /Open item/ })).toHaveAttribute("href", "?item=item0002abcdef");
    expect(first).toHaveTextContent("yes (c3)");
  });

  it("DeadlockTable_should_SayNoStreaks_When_Empty", () => {
    render(<DeadlockTable page={page(0)} weakFilter={WEAK} estimated />);

    expect(screen.getByText(/No review-fail streaks in this window/)).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("Tables_should_HaveCaptionScopeAndLabelledScrollRegion_When_DeadlockRendered", () => {
    render(<DeadlockTable page={page(2)} weakFilter={WEAK} estimated />);

    expect(screen.getByRole("table", { name: /Review-fail streaks/ })).toBeInTheDocument();
    screen.getAllByRole("columnheader").forEach((th) => expect(th).toHaveAttribute("scope", "col"));
    expect(screen.getByRole("region", { name: /Review-fail streaks/ })).toHaveAttribute("tabindex", "0");
  });
});
