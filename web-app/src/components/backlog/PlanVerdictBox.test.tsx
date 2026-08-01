/**
 * Tests for PlanVerdictBox component.
 *
 * Covers:
 *  1. Each of the 5 statuses renders a matching icon+label pair
 *  2. role="status" aria-live="polite" aria-atomic="true" on the root
 *  3. Reject submit is disabled until the reason textarea is non-empty
 *  4. Submitting a valid reason calls onReject with the trimmed text
 *  5. "Regenerate Plan with This Feedback" only renders in changes_requested state
 *  6. Cancel returns focus to the "Request Changes" toggle button
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { PlanVerdictBox, type PlanVerdictBoxProps } from "./PlanVerdictBox";
import type { PlanReviewStatus } from "@/lib/backlog/planReviewStatus";

function makeProps(overrides: Partial<PlanVerdictBoxProps> = {}): PlanVerdictBoxProps {
  return {
    status: "pending_review",
    onReject: jest.fn().mockResolvedValue(undefined),
    onRegenerateWithFeedback: jest.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

describe("PlanVerdictBox — status rendering", () => {
  const cases: Array<{ status: PlanReviewStatus; label: string }> = [
    { status: "no_plan", label: "No plan yet" },
    { status: "pending_review", label: "Pending review" },
    { status: "approved", label: "Plan approved" },
    { status: "changes_requested", label: "Changes requested" },
    { status: "skipped", label: "Planning skipped" },
  ];

  it.each(cases)("renders the matching icon+label pair for $status", ({ status, label }) => {
    render(<PlanVerdictBox {...makeProps({ status })} />);
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  it("sets role=status aria-live=polite aria-atomic=true on the root", () => {
    render(<PlanVerdictBox {...makeProps()} />);
    const el = screen.getByRole("status");
    expect(el).toHaveAttribute("aria-live", "polite");
    expect(el).toHaveAttribute("aria-atomic", "true");
  });
});

describe("PlanVerdictBox — reject form", () => {
  it("disables submit while the reason textarea is empty or whitespace", () => {
    render(<PlanVerdictBox {...makeProps({ status: "pending_review" })} />);
    fireEvent.click(screen.getByTestId("backlog-action-reject-plan"));

    const submit = screen.getByTestId("backlog-action-reject-plan-submit");
    expect(submit).toBeDisabled();

    fireEvent.change(screen.getByTestId("plan-reject-reason"), { target: { value: "   " } });
    expect(submit).toBeDisabled();

    fireEvent.change(screen.getByTestId("plan-reject-reason"), { target: { value: "missing caching plan" } });
    expect(submit).not.toBeDisabled();
  });

  it("calls onReject with the trimmed reason on submit", async () => {
    const onReject = jest.fn().mockResolvedValue(undefined);
    render(<PlanVerdictBox {...makeProps({ status: "pending_review", onReject })} />);
    fireEvent.click(screen.getByTestId("backlog-action-reject-plan"));
    fireEvent.change(screen.getByTestId("plan-reject-reason"), { target: { value: "  needs work  " } });
    fireEvent.click(screen.getByTestId("backlog-action-reject-plan-submit"));

    await waitFor(() => expect(onReject).toHaveBeenCalledWith("needs work"));
  });

  it("returns focus to the Request Changes toggle button on Cancel", () => {
    render(<PlanVerdictBox {...makeProps({ status: "pending_review" })} />);
    const toggle = screen.getByTestId("backlog-action-reject-plan");
    fireEvent.click(toggle);
    fireEvent.click(screen.getByText("Cancel"));
    expect(toggle).toHaveFocus();
  });
});

describe("PlanVerdictBox — regenerate button", () => {
  it("only renders in changes_requested state", () => {
    const { rerender } = render(<PlanVerdictBox {...makeProps({ status: "pending_review" })} />);
    expect(screen.queryByTestId("backlog-action-regenerate-plan")).not.toBeInTheDocument();

    rerender(<PlanVerdictBox {...makeProps({ status: "changes_requested", rejectionReason: "fix it" })} />);
    expect(screen.getByTestId("backlog-action-regenerate-plan")).toBeInTheDocument();
  });

  it("shows the persisted rejection reason in changes_requested state", () => {
    render(<PlanVerdictBox {...makeProps({ status: "changes_requested", rejectionReason: "missing caching plan" })} />);
    expect(screen.getByTestId("plan-rejection-reason")).toHaveTextContent("missing caching plan");
  });
});
