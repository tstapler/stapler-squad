import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { AcCriteriaList } from "./AcCriteriaList";

const criteria = [
  { index: 0, text: "builds", status: "done" as const },
  { index: 1, text: "mac proof", status: "pending" as const },
  { index: 2, text: "linux proof", status: "waived" as const, waiverReason: "no hardware" },
];

describe("AcCriteriaList waivers", () => {
  it("renders a waived criterion distinctly with its reason", () => {
    render(<AcCriteriaList criteria={criteria} />);
    expect(screen.getByTestId("ac-waived-chip")).toHaveTextContent("Waived");
    expect(screen.getByText("Waiver reason: no hardware")).toBeInTheDocument();
  });

  it("shows who waived and when, with a fallback for legacy waivers", () => {
    const { rerender } = render(<AcCriteriaList criteria={criteria} />);
    expect(screen.getByTestId("ac-waived-by-2")).toHaveTextContent(
      "Waived by unknown caller (recorded before audit trail) at unknown time (identity not verified)"
    );
    rerender(
      <AcCriteriaList
        criteria={[{ ...criteria[2], waivedBy: "claimed agent session abc (unverified claim)", waivedAt: "2026-11-05T12:00:00Z" }]}
      />
    );
    const line = screen.getByTestId("ac-waived-by-2").textContent ?? "";
    expect(line).toContain("Waived by claimed agent session abc (unverified claim) at ");
    expect(line).not.toContain("unknown time");
    expect(line).toContain("identity not verified");
  });

  it("offers Waive only on unmet criteria and requires a non-blank reason", async () => {
    const onWaive = jest.fn().mockResolvedValue(undefined);
    render(<AcCriteriaList criteria={criteria} onWaive={onWaive} />);
    expect(screen.queryByTestId("ac-waive-0")).not.toBeInTheDocument();
    expect(screen.queryByTestId("ac-waive-2")).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("ac-waive-1"));
    expect(screen.getByTestId("ac-waive-confirm-1")).toBeDisabled();
    fireEvent.change(screen.getByTestId("ac-waive-reason-1"), { target: { value: "  " } });
    expect(screen.getByTestId("ac-waive-confirm-1")).toBeDisabled();
    fireEvent.change(screen.getByTestId("ac-waive-reason-1"), { target: { value: "waived verbally" } });
    fireEvent.click(screen.getByTestId("ac-waive-confirm-1"));
    await waitFor(() => expect(onWaive).toHaveBeenCalledWith(1, "waived verbally"));
  });
});
