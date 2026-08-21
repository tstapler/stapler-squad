import React from "react";
import { render, screen } from "@testing-library/react";
import { BlockedNotice } from "./BlockedNotice";

describe("BlockedNotice_should_RenderReviewVerdictSummaryVerbatimUnderRoleStatus_When_KindIsBlockedGuardrail", () => {
  it("renders the review-blocked-* summary verbatim inside a role=status region, with no open-session affordance", () => {
    render(
      <BlockedNotice
        kind="blocked_guardrail"
        session={{
          reviewVerdict: {
            overallOutcome: "FAIL",
            summary:
              "Review blocked by security check: secret pattern detected: aws_access_key_id. Override required to proceed.",
          },
        }}
      />
    );

    const notice = screen.getByRole("status");
    expect(notice).toHaveTextContent(
      "Review blocked by security check: secret pattern detected: aws_access_key_id. Override required to proceed."
    );
    expect(screen.getByText("Blocked before starting")).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("renders the diff-error-* summary verbatim under the same blocked_guardrail treatment", () => {
    render(
      <BlockedNotice
        kind="blocked_guardrail"
        session={{
          reviewVerdict: {
            overallOutcome: "FAIL",
            summary:
              "Review blocked: could not compute a diff for this session (base commit missing). This needs investigation, not rework.",
          },
        }}
      />
    );

    expect(screen.getByRole("status")).toHaveTextContent(/could not compute a diff/);
    expect(screen.getByText("Blocked before starting")).toBeInTheDocument();
  });

  it("renders the manual-review-* summary under the distinct 'Manual review' label", () => {
    render(
      <BlockedNotice
        kind="manual_review_marker"
        session={{
          reviewVerdict: { overallOutcome: "PASS", summary: "Manual review: verified fix locally" },
        }}
      />
    );

    expect(screen.getByRole("status")).toHaveTextContent("Manual review: verified fix locally");
    expect(screen.getByText("Manual review")).toBeInTheDocument();
    expect(screen.queryByText("Blocked before starting")).not.toBeInTheDocument();
  });
});

describe("BlockedNotice_should_RenderNoSummaryRecordedFallback_When_ReviewVerdictSummaryIsUndefined", () => {
  it("renders 'No summary recorded.' rather than a blank box when reviewVerdict.summary is undefined", () => {
    render(<BlockedNotice kind="blocked_guardrail" session={{ reviewVerdict: { overallOutcome: "FAIL" } }} />);

    expect(screen.getByRole("status")).toHaveTextContent("No summary recorded.");
  });

  it("renders 'No summary recorded.' when reviewVerdict itself is undefined", () => {
    render(<BlockedNotice kind="manual_review_marker" session={{}} />);

    expect(screen.getByRole("status")).toHaveTextContent("No summary recorded.");
  });

  it("renders the distinct 'No diagnostic data recorded.' fallback for the missing_diagnostic_data kind", () => {
    render(<BlockedNotice kind="missing_diagnostic_data" session={{}} />);

    expect(screen.getByRole("status")).toHaveTextContent("No diagnostic data recorded.");
    expect(screen.getByText("No diagnostic data")).toBeInTheDocument();
  });
});

describe("BlockedNotice_should_RenderHeadlessFailureDetail_When_EndReasonIsPopulated", () => {
  it("renders the classified failure reason and capture path instead of the generic fallback", () => {
    render(
      <BlockedNotice
        kind="missing_diagnostic_data"
        session={{ endReason: "timeout", failureCapturePath: "/home/user/.stapler-squad/headless-failures/headless-failure-abc.txt" }}
      />
    );

    const notice = screen.getByRole("status");
    expect(notice).toHaveTextContent("Headless call failed (timeout).");
    expect(notice).toHaveTextContent("/home/user/.stapler-squad/headless-failures/headless-failure-abc.txt");
    expect(screen.queryByText("No diagnostic data recorded.")).not.toBeInTheDocument();
  });

  it("renders the failure reason alone when no capture path was recorded", () => {
    render(<BlockedNotice kind="missing_diagnostic_data" session={{ endReason: "claude_not_found" }} />);

    expect(screen.getByRole("status")).toHaveTextContent("Headless call failed (claude_not_found).");
  });

  it("still prefers reviewVerdict.summary over endReason when both happen to be present", () => {
    render(
      <BlockedNotice
        kind="blocked_guardrail"
        session={{
          reviewVerdict: { overallOutcome: "FAIL", summary: "Review blocked: explicit summary" },
          endReason: "timeout",
        }}
      />
    );

    expect(screen.getByRole("status")).toHaveTextContent("Review blocked: explicit summary");
    expect(screen.queryByText(/Headless call failed/)).not.toBeInTheDocument();
  });
});
