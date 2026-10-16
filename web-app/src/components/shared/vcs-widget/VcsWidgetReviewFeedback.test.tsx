import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { VcsWidgetReviewFeedback } from "./VcsWidgetReviewFeedback";
import type { ReviewFeedbackSummary } from "@/lib/vcs/types";

function makeReview(overrides: Partial<ReviewFeedbackSummary> = {}): ReviewFeedbackSummary {
  return {
    author: "octocat",
    state: "CHANGES_REQUESTED",
    body: "Please fix the thing.",
    ...overrides,
  };
}

describe("VcsWidgetReviewFeedback", () => {
  it("VcsWidgetReviewFeedback_should_RenderNothing_When_ReviewFeedbackEmpty", () => {
    const { container } = render(<VcsWidgetReviewFeedback reviewFeedback={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("VcsWidgetReviewFeedback_should_RenderNothing_When_NoReviewIsChangesRequested", () => {
    const reviews = [makeReview({ state: "APPROVED" }), makeReview({ state: "COMMENTED" })];
    const { container } = render(<VcsWidgetReviewFeedback reviewFeedback={reviews} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("VcsWidgetReviewFeedback_should_RenderAuthorAndBody_When_ChangesRequestedReviewPresent", () => {
    const reviews = [
      makeReview({ author: "reviewer-1", state: "CHANGES_REQUESTED", body: "Needs more tests." }),
    ];
    render(<VcsWidgetReviewFeedback reviewFeedback={reviews} />);

    fireEvent.click(screen.getByTestId("collapsible-header-review-feedback"));

    expect(screen.getByText("reviewer-1")).toBeInTheDocument();
    expect(screen.getByText("Needs more tests.")).toBeInTheDocument();
  });

  it("VcsWidgetReviewFeedback_should_OmitApprovedReview_When_MixedWithChangesRequested", () => {
    const reviews = [
      makeReview({ author: "approver", state: "APPROVED", body: "Looks great!" }),
      makeReview({ author: "blocker", state: "CHANGES_REQUESTED", body: "Fix this." }),
    ];
    render(<VcsWidgetReviewFeedback reviewFeedback={reviews} />);

    fireEvent.click(screen.getByTestId("collapsible-header-review-feedback"));

    expect(screen.queryByText("approver")).not.toBeInTheDocument();
    expect(screen.queryByText("Looks great!")).not.toBeInTheDocument();
    expect(screen.getByText("blocker")).toBeInTheDocument();
    expect(screen.getByText("Fix this.")).toBeInTheDocument();
  });

  it("VcsWidgetReviewFeedback_should_StartCollapsed_When_Rendered", () => {
    render(<VcsWidgetReviewFeedback reviewFeedback={[makeReview()]} />);

    expect(screen.getByTestId("collapsible-header-review-feedback")).toHaveAttribute(
      "aria-expanded",
      "false"
    );
  });

  it("VcsWidgetReviewFeedback_should_RenderRawHtmlAsLiteralText_When_ReviewBodyContainsHtmlTags", () => {
    const reviews = [
      makeReview({
        author: "attacker",
        state: "CHANGES_REQUESTED",
        body: '<img src=x onerror=alert(1)>',
      }),
    ];
    const { container } = render(<VcsWidgetReviewFeedback reviewFeedback={reviews} />);

    fireEvent.click(screen.getByTestId("collapsible-header-review-feedback"));

    // The <img> tag must never be parsed into a real DOM element.
    expect(container.querySelector("img")).toBeNull();
    // The literal, escaped text must still be present as text content.
    expect(screen.getByText("<img src=x onerror=alert(1)>")).toBeInTheDocument();
  });
});
