/**
 * Tests for PlanVerdictBox (Story 4.2.1).
 *
 * Covers:
 *  1. Renders the correct icon+label per status (5 cases)
 *  2. role="status" aria-live="polite" aria-atomic="true" present
 *  3. Reject submit disabled until reason is non-empty (attribute guard)
 *  4. "Regenerate" button only appears in changes_requested state
 *  5. Cancel returns focus to the "Request Changes" toggle button
 *  6. Submitting a reason calls onReject with the typed text
 *  7. Regenerate button calls onRegenerateWithFeedback
 *  8. readOnly hides both the reject toggle and the regenerate button
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
  const cases: Array<{ status: PlanReviewStatus; icon: string; label: string }> = [
    { status: "no_plan", icon: "○", label: "No plan yet" },
    { status: "pending_review", icon: "◌", label: "Pending review" },
    { status: "approved", icon: "✓", label: "Plan approved" },
    { status: "changes_requested", icon: "✎", label: "Revisions requested" },
    { status: "skipped", icon: "⊘", label: "Planning skipped" },
  ];

  it.each(cases)(
    "PlanVerdictBox_should_RenderIconAndLabel_When_StatusIs$status",
    ({ status, icon, label }) => {
      render(<PlanVerdictBox {...makeProps({ status })} />);
      expect(screen.getByText(label)).toBeInTheDocument();
      expect(screen.getByText(icon)).toBeInTheDocument();
    },
  );

  it("renders role=status with aria-live=polite and aria-atomic=true", () => {
    render(<PlanVerdictBox {...makeProps()} />);
    const region = screen.getByRole("status", { name: /Plan review status/i });
    expect(region).toHaveAttribute("aria-live", "polite");
    expect(region).toHaveAttribute("aria-atomic", "true");
  });
});

describe("PlanVerdictBox — Request Changes form", () => {
  it("submit button is disabled until a non-empty reason is typed", () => {
    render(<PlanVerdictBox {...makeProps({ status: "pending_review" })} />);

    fireEvent.click(screen.getByTestId("backlog-action-reject-plan"));

    const submitBtn = screen.getByTestId("backlog-action-reject-plan-submit");
    expect(submitBtn).toBeDisabled();
    expect(submitBtn).toHaveAttribute("aria-disabled", "true");

    const textarea = screen.getByTestId("plan-reject-reason");
    fireEvent.change(textarea, { target: { value: "   " } });
    expect(submitBtn).toBeDisabled();
    expect(submitBtn).toHaveAttribute("aria-disabled", "true");

    fireEvent.change(textarea, { target: { value: "Needs a different approach." } });
    expect(submitBtn).not.toBeDisabled();
    expect(submitBtn).toHaveAttribute("aria-disabled", "false");
  });

  it("submitting a non-empty reason calls onReject with the typed text", async () => {
    const onReject = jest.fn().mockResolvedValue(undefined);
    render(<PlanVerdictBox {...makeProps({ status: "pending_review", onReject })} />);

    fireEvent.click(screen.getByTestId("backlog-action-reject-plan"));
    fireEvent.change(screen.getByTestId("plan-reject-reason"), {
      target: { value: "Please split this into two smaller tasks." },
    });
    fireEvent.click(screen.getByTestId("backlog-action-reject-plan-submit"));

    await waitFor(() => expect(onReject).toHaveBeenCalledTimes(1));
    expect(onReject).toHaveBeenCalledWith("Please split this into two smaller tasks.");
  });

  it("Cancel returns focus to the Request Changes toggle button", () => {
    render(<PlanVerdictBox {...makeProps({ status: "pending_review" })} />);

    const toggle = screen.getByTestId("backlog-action-reject-plan");
    fireEvent.click(toggle);
    expect(screen.getByTestId("plan-reject-reason")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /^Cancel$/i }));
    expect(screen.queryByTestId("plan-reject-reason")).not.toBeInTheDocument();
    expect(screen.getByTestId("backlog-action-reject-plan")).toHaveFocus();
  });

  it("Escape in the textarea cancels the form", () => {
    render(<PlanVerdictBox {...makeProps({ status: "pending_review" })} />);

    fireEvent.click(screen.getByTestId("backlog-action-reject-plan"));
    const textarea = screen.getByTestId("plan-reject-reason");
    fireEvent.keyDown(textarea, { key: "Escape" });

    expect(screen.queryByTestId("plan-reject-reason")).not.toBeInTheDocument();
  });
});

describe("PlanVerdictBox — Regenerate with feedback", () => {
  it("only renders the Regenerate button when status is changes_requested", () => {
    render(<PlanVerdictBox {...makeProps({ status: "pending_review" })} />);
    expect(screen.queryByTestId("backlog-action-regenerate-plan")).not.toBeInTheDocument();
  });

  it("renders the Regenerate button and reason text when status is changes_requested", async () => {
    const onRegenerateWithFeedback = jest.fn().mockResolvedValue(undefined);
    render(
      <PlanVerdictBox
        {...makeProps({
          status: "changes_requested",
          rejectionReason: "Needs a different approach.",
          onRegenerateWithFeedback,
        })}
      />,
    );

    expect(screen.getByText("Needs a different approach.")).toBeInTheDocument();
    const regenBtn = screen.getByTestId("backlog-action-regenerate-plan");
    expect(regenBtn).toBeInTheDocument();

    fireEvent.click(regenBtn);
    await waitFor(() => expect(onRegenerateWithFeedback).toHaveBeenCalledTimes(1));
  });
});

describe("PlanVerdictBox — readOnly mode", () => {
  it("hides the reject toggle and the regenerate button when readOnly", () => {
    render(
      <PlanVerdictBox
        {...makeProps({
          status: "changes_requested",
          rejectionReason: "Needs a different approach.",
          readOnly: true,
        })}
      />,
    );

    expect(screen.queryByTestId("backlog-action-reject-plan")).not.toBeInTheDocument();
    expect(screen.queryByTestId("backlog-action-regenerate-plan")).not.toBeInTheDocument();
    // Status + reason still render read-only.
    expect(screen.getByText("Revisions requested")).toBeInTheDocument();
    expect(screen.getByText("Needs a different approach.")).toBeInTheDocument();
  });
});
