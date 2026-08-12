// +feature: escape-analytics

import { render, screen, within } from "@testing-library/react";
import type { SessionEscapeSummary } from "@/gen/session/v1/session_pb";
import { SessionEscapeBreakdownTable } from "./SessionEscapeBreakdownTable";

function makeRow(overrides: Partial<SessionEscapeSummary>): SessionEscapeSummary {
  return {
    sessionId: "session-a",
    totalSequences: 100n,
    totalMangled: 1n,
    mangleRate: 0.01,
    ...overrides,
  } as SessionEscapeSummary;
}

describe("SessionEscapeBreakdownTable", () => {
  it("SessionEscapeBreakdownTable_should_SortDescendingByMangleRate_When_DefaultRendered", () => {
    const rows: SessionEscapeSummary[] = [
      makeRow({ sessionId: "session-low", totalSequences: 100n, totalMangled: 1n, mangleRate: 0.01 }),
      makeRow({ sessionId: "session-high", totalSequences: 100n, totalMangled: 40n, mangleRate: 0.4 }),
      makeRow({ sessionId: "session-mid", totalSequences: 100n, totalMangled: 10n, mangleRate: 0.1 }),
    ];

    render(
      <SessionEscapeBreakdownTable rows={rows} fleetMangleRate={0.05} />
    );

    const dataRows = screen.getAllByTestId("session-escape-breakdown-row");
    expect(dataRows).toHaveLength(3);
    expect(within(dataRows[0]).getByText("session-high")).toBeInTheDocument();
    expect(within(dataRows[1]).getByText("session-mid")).toBeInTheDocument();
    expect(within(dataRows[2]).getByText("session-low")).toBeInTheDocument();

    const mangleRateHeader = screen.getByTestId("sort-button-mangleRate").closest("th");
    expect(mangleRateHeader).toHaveAttribute("aria-sort", "descending");
  });

  it("SessionEscapeBreakdownTable_should_ApplyNonColorOutlierCue_When_RowExceedsThreshold", () => {
    const rows: SessionEscapeSummary[] = [
      makeRow({ sessionId: "session-normal", totalSequences: 100n, totalMangled: 1n, mangleRate: 0.01 }),
      makeRow({ sessionId: "session-outlier", totalSequences: 100n, totalMangled: 50n, mangleRate: 0.5 }),
    ];

    render(
      <SessionEscapeBreakdownTable rows={rows} fleetMangleRate={0.01} />
    );

    const dataRows = screen.getAllByTestId("session-escape-breakdown-row");
    const outlierRow = dataRows.find((row) => row.getAttribute("data-outlier") === "true");
    const normalRow = dataRows.find((row) => row.getAttribute("data-outlier") === "false");

    expect(outlierRow).toBeDefined();
    expect(normalRow).toBeDefined();
    if (!outlierRow || !normalRow) throw new Error("expected both an outlier and a normal row");

    // Non-color cue: a visible glyph plus visually-hidden text must be present
    // on the outlier row independent of any CSS background tint (WCAG 1.4.1).
    expect(within(outlierRow).getByText("⚠")).toBeInTheDocument();
    expect(within(outlierRow).getByText("Outlier:")).toBeInTheDocument();

    expect(within(normalRow).queryByText("⚠")).not.toBeInTheDocument();
    expect(within(normalRow).queryByText("Outlier:")).not.toBeInTheDocument();
  });
});
